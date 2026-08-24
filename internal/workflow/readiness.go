package workflow

import (
	"context"
	"strings"

	"archive-review/internal/domain"
	"archive-review/internal/redaction"
)

func (s *Service) ReviewReadiness(ctx context.Context, caseID, actorID, reviewerID string) (*domain.ReviewReadiness, error) {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return nil, domain.Invalid("actor_id", "操作者请求头不能为空")
	}
	c, err := s.repo.Get(ctx, caseID)
	if err != nil {
		return nil, err
	}
	if err := requireCaseActor(c, actorID); err != nil {
		return nil, err
	}
	result := &domain.ReviewReadiness{PreflightRevision: c.Revision, Blockers: []domain.ReadinessBlocker{}}
	if !c.Status.Editable() {
		result.Blockers = append(result.Blockers, domain.ReadinessBlocker{Field: "status", Code: "invalid_state", Message: "当前状态不能提交复核"})
	}
	reviewerID = strings.TrimSpace(reviewerID)
	if reviewerID == "" {
		result.Blockers = append(result.Blockers, domain.ReadinessBlocker{Field: "reviewer_id", Code: "required", Message: "候选复核人不能为空"})
	} else if reviewerID == c.SubmitterID {
		result.Blockers = append(result.Blockers, domain.ReadinessBlocker{Field: "reviewer_id", Code: "not_independent", Message: "提交人与复核人不得相同"})
	}
	counts := domain.DecisionCounts{}
	allDecided := true
	for _, finding := range c.Findings {
		if err := finding.Validate(len(c.ContentExcerpt)); err != nil {
			result.Blockers = append(result.Blockers, domain.ReadinessBlocker{Field: "findings", FindingID: finding.ID,
				Code: "finding_invalid", Message: err.Error()})
			allDecided = false
		}
		switch finding.Decision {
		case domain.DecisionPending:
			counts.Pending++
			allDecided = false
			result.Blockers = append(result.Blockers, domain.ReadinessBlocker{Field: "findings", FindingID: finding.ID, Code: "pending", Message: "发现项尚未处理"})
		case domain.DecisionAccept:
			counts.Accepted++
		case domain.DecisionAdjust:
			counts.Adjusted++
		case domain.DecisionReject:
			counts.Rejected++
		}
		if finding.IsDecided() && strings.TrimSpace(finding.DecisionReason) == "" {
			result.Blockers = append(result.Blockers, domain.ReadinessBlocker{Field: "reason", FindingID: finding.ID, Code: "required", Message: "发现项处理理由缺失"})
		}
	}
	if err := redaction.ValidateLayout(c.ContentExcerpt, c.Findings); err != nil {
		result.Blockers = append(result.Blockers, domain.ReadinessBlocker{Field: "findings", Code: "render_invalid", Message: err.Error()})
		allDecided = false
	}
	if len(c.Reviews) > 0 && c.Reviews[len(c.Reviews)-1].Outcome == domain.ReviewReturned {
		returned := c.Reviews[len(c.Reviews)-1]
		for _, item := range returned.RemediationItems {
			for _, finding := range c.Findings {
				if finding.ID == item.FindingID && (!finding.IsDecided() || finding.DecisionRevision <= returned.CaseRevision) {
					result.Blockers = append(result.Blockers, domain.ReadinessBlocker{Field: "findings", FindingID: finding.ID,
						Code: "remediation_not_updated", Message: "退回整改项尚未产生新的决定"})
				}
			}
		}
	}
	if allDecided {
		rendered, renderErr := redaction.Render(c.ContentExcerpt, c.Findings)
		if renderErr != nil {
			result.Blockers = append(result.Blockers, domain.ReadinessBlocker{Field: "findings", Code: "render_invalid", Message: renderErr.Error()})
		} else if len(result.Blockers) == 0 {
			result.Evidence = &domain.ReviewEvidenceSummary{Decisions: counts, ContentFingerprint: domain.DigestString(rendered)}
		}
	}
	result.Ready = len(result.Blockers) == 0
	return result, nil
}
