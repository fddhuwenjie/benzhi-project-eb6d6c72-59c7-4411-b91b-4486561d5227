package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"archive-review/internal/domain"
	"archive-review/internal/redaction"
)

func (s *Service) AuditIntegrity(ctx context.Context, caseID string) (*domain.IntegrityResult, error) {
	c, err := s.repo.Get(ctx, caseID)
	if err != nil {
		if domain.ErrorCodeOf(err) == domain.CodeNotFound || domain.ErrorCodeOf(err) == domain.CodeInvalid {
			return nil, err
		}
		return &domain.IntegrityResult{Passed: false, Verifiable: false, Issues: []domain.IntegrityIssue{{
			Code: "snapshot_unreadable", Field: "snapshot", Message: err.Error(),
		}}}, nil
	}
	result := &domain.IntegrityResult{Verifiable: true, SnapshotRevision: c.Revision, Issues: []domain.IntegrityIssue{}}
	events, err := s.repo.Events(ctx, caseID)
	if err != nil {
		result.Verifiable = false
		result.Issues = append(result.Issues, domain.IntegrityIssue{Code: "audit_unreadable", Field: "audit_events", Message: err.Error()})
		return result, nil
	}
	if len(events) == 0 {
		result.Verifiable = false
		result.Issues = append(result.Issues, domain.IntegrityIssue{Code: "audit_empty", Field: "audit_events", Message: "案件没有可验证的审计事件"})
		return result, nil
	}
	result.FirstSequence, result.LastSequence = events[0].Sequence, events[len(events)-1].Sequence
	add := func(code, field, message string, sequence int64) {
		result.Issues = append(result.Issues, domain.IntegrityIssue{Code: code, Field: field, Sequence: sequence, Message: message})
	}
	if events[0].EventType != "case.created" || events[0].CaseRevision != 1 {
		add("invalid_first_event", "audit_events", "首事件必须是 revision 1 的案件创建事件", events[0].Sequence)
	}
	requests := map[string]int64{}
	decisionEvidence := map[string]bool{}
	reviewEvents := 0
	for i, event := range events {
		if event.CaseID != caseID {
			add("case_id_mismatch", "case_id", "审计事件案件标识不一致", event.Sequence)
		}
		if previous, exists := requests[event.RequestID]; exists {
			add("duplicate_request_id", "request_id", fmt.Sprintf("请求标识与序列 %d 重复", previous), event.Sequence)
		} else {
			requests[event.RequestID] = event.Sequence
		}
		if event.CaseRevision != int64(i+1) {
			add("revision_gap", "case_revision", fmt.Sprintf("事件 revision 应为 %d，实际为 %d", i+1, event.CaseRevision), event.Sequence)
		}
		if i > 0 {
			previous := events[i-1]
			if event.Sequence <= previous.Sequence {
				add("sequence_order", "sequence", "审计序列未严格递增", event.Sequence)
			}
			if event.FromStatus != previous.ToStatus {
				add("state_disconnected", "status", "相邻审计事件状态未衔接", event.Sequence)
			}
			if event.FromStatus != event.ToStatus && !domain.CanTransition(event.FromStatus, event.ToStatus) {
				add("invalid_transition", "status", "审计事件包含非法状态迁移", event.Sequence)
			}
		}
		if len(event.Payload) == 0 {
			add("payload_missing", "payload", "关键写事件缺少审计载荷", event.Sequence)
		} else {
			var payload any
			if json.Unmarshal(event.Payload, &payload) != nil || domain.DigestString(string(event.Payload)) != event.PayloadDigest {
				add("payload_digest_mismatch", "payload_digest", "审计载荷摘要校验失败", event.Sequence)
			}
		}
		switch event.EventType {
		case "finding.decided":
			var payload struct {
				FindingID string `json:"finding_id"`
			}
			if json.Unmarshal(event.Payload, &payload) == nil {
				decisionEvidence[payload.FindingID] = true
			}
		case "findings.batch_decided":
			var payload struct {
				Items []struct {
					FindingID string `json:"finding_id"`
				} `json:"items"`
			}
			if json.Unmarshal(event.Payload, &payload) == nil {
				for _, item := range payload.Items {
					decisionEvidence[item.FindingID] = true
				}
			}
		case "review.returned", "review.approved":
			reviewEvents++
		}
	}
	if int64(len(events)) != c.Revision {
		add("revision_count_mismatch", "revision", "审计写事件数量无法解释案件 revision", events[len(events)-1].Sequence)
	}
	if events[len(events)-1].ToStatus != c.Status {
		add("snapshot_status_mismatch", "status", "末事件状态与当前快照不一致", events[len(events)-1].Sequence)
	}
	if reviewEvents != len(c.Reviews) {
		add("review_evidence_mismatch", "reviews", "复核写事件数量与快照复核记录不一致", 0)
	}
	for _, finding := range c.Findings {
		if finding.IsDecided() && !decisionEvidence[finding.ID] {
			add("decision_evidence_missing", "findings", "已处理发现项缺少决定事件证据: "+finding.ID, 0)
		}
	}
	if c.Status == domain.StatusPublished && c.Manifest != nil {
		recomputed, buildErr := redaction.BuildManifest(c)
		if buildErr != nil {
			add("manifest_unverifiable", "manifest", buildErr.Error(), 0)
			result.Verifiable = false
		} else {
			result.ContentFingerprint, result.ManifestDigest = recomputed.ContentFingerprint, recomputed.ManifestDigest
			if c.Manifest.ContentFingerprint != recomputed.ContentFingerprint {
				add("content_fingerprint_mismatch", "manifest.content_fingerprint", "冻结内容指纹与重新计算结果不一致", 0)
			}
			if c.Manifest.ManifestDigest != recomputed.ManifestDigest {
				add("manifest_digest_mismatch", "manifest.manifest_digest", "冻结清单摘要与重新计算结果不一致", 0)
			}
			frozen, _ := json.Marshal(c.Manifest)
			current, _ := json.Marshal(recomputed)
			if !bytes.Equal(frozen, current) && c.Manifest.ContentFingerprint == recomputed.ContentFingerprint && c.Manifest.ManifestDigest == recomputed.ManifestDigest {
				add("manifest_bytes_mismatch", "manifest", "冻结发布清单与重新生成清单不一致", 0)
			}
		}
	}
	result.Passed = result.Verifiable && len(result.Issues) == 0
	return result, nil
}
