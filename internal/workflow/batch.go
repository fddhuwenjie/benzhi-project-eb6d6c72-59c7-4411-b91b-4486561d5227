package workflow

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"archive-review/internal/domain"
	"archive-review/internal/redaction"
)

const opBatchDecide = "finding.batch_decide"

type BatchFindingDecision struct {
	FindingID   string                 `json:"finding_id"`
	Decision    domain.FindingDecision `json:"decision"`
	Replacement string                 `json:"replacement,omitempty"`
	Reason      string                 `json:"reason"`
}

type BatchDecisionInput struct {
	Decisions []BatchFindingDecision `json:"decisions"`
}

func (s *Service) DecideFindingsBatch(ctx context.Context, caseID string, meta WriteMeta, input BatchDecisionInput) (*domain.DisclosureCase, error) {
	if err := meta.Validate(true); err != nil {
		return nil, err
	}
	if len(input.Decisions) == 0 || len(input.Decisions) > 100 {
		return nil, domain.Invalid("decisions", "批量决定数量必须为 1 到 100 项")
	}
	normalized := append([]BatchFindingDecision(nil), input.Decisions...)
	for i := range normalized {
		normalized[i].FindingID = strings.TrimSpace(normalized[i].FindingID)
		normalized[i].Reason = strings.TrimSpace(normalized[i].Reason)
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].FindingID < normalized[j].FindingID })
	for i := range normalized {
		if normalized[i].FindingID == "" {
			return nil, domain.Invalid(fmt.Sprintf("decisions[%d].finding_id", i), "发现项标识不能为空")
		}
		if i > 0 && normalized[i].FindingID == normalized[i-1].FindingID {
			return nil, domain.Invalid("decisions", "同一批次不得包含重复发现项标识")
		}
	}
	operation := opBatchDecide + ":" + domain.PayloadDigest(normalized)
	if replay, ok, err := s.replay(ctx, operation, caseID, meta.RequestID); ok || err != nil {
		return replay, err
	}
	c, err := s.repo.Get(ctx, caseID)
	if err != nil {
		return nil, err
	}
	if err := requireCaseActor(c, meta.ActorID); err != nil {
		return nil, err
	}
	if !c.Status.Editable() {
		return nil, domain.InvalidState("当前状态不允许批量处理发现项")
	}
	if c.Revision != meta.ExpectedRevision {
		return nil, domain.NewError(domain.CodeConflict, "案件 revision 已变化", "revision")
	}
	indices := make(map[string]int, len(c.Findings))
	for i := range c.Findings {
		indices[c.Findings[i].ID] = i
	}
	now := s.clock()
	for i, decision := range normalized {
		index, ok := indices[decision.FindingID]
		if !ok || c.Findings[index].CaseID != caseID {
			return nil, domain.Invalid(fmt.Sprintf("decisions[%d].finding_id", i), "发现项不属于当前案件")
		}
		if c.Findings[index].IsDecided() {
			return nil, domain.Invalid(fmt.Sprintf("decisions[%d].finding_id", i), "发现项已经完成决策")
		}
		if err := c.Findings[index].Decide(decision.Decision, decision.Replacement, decision.Reason, meta.ActorID, now); err != nil {
			if problem, ok := err.(*domain.Error); ok {
				return nil, domain.NewError(problem.Code, problem.Message, fmt.Sprintf("decisions[%d].%s", i, problem.Field))
			}
			return nil, err
		}
	}
	if c.AllFindingsDecided() {
		if _, err := redaction.Render(c.ContentExcerpt, c.Findings); err != nil {
			return nil, err
		}
	} else if err := redaction.ValidateLayout(c.ContentExcerpt, c.Findings); err != nil {
		return nil, err
	}
	from := c.Status
	c.Touch(s.clock())
	for _, decision := range normalized {
		c.Findings[indices[decision.FindingID]].DecisionRevision = c.Revision
	}
	detectionRevision := c.Revision
	if c.RiskSummary != nil {
		detectionRevision = c.RiskSummary.DetectionRevision
	}
	c.RiskSummary, err = redaction.BuildRiskSummaryForContent(c.ContentExcerpt, c.Findings, detectionRevision, c.Revision)
	if err != nil {
		return nil, err
	}
	type itemSummary struct {
		FindingID    string                 `json:"finding_id"`
		Decision     domain.FindingDecision `json:"decision"`
		ReasonDigest string                 `json:"reason_digest"`
	}
	items := make([]itemSummary, 0, len(normalized))
	for _, decision := range normalized {
		items = append(items, itemSummary{decision.FindingID, decision.Decision, domain.DigestString(decision.Reason)})
	}
	event := s.event(c, "findings.batch_decided", meta, from, struct {
		Count int           `json:"count"`
		Items []itemSummary `json:"items"`
	}{len(items), items})
	if err := s.commit(ctx, operation, c, meta.ExpectedRevision, event, meta.RequestID, false); err != nil {
		return nil, err
	}
	return c.Clone()
}
