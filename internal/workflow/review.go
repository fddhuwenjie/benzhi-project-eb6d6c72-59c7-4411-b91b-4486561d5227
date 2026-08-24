package workflow

import (
	"context"
	"strings"

	"archive-review/internal/domain"
	"archive-review/internal/redaction"
)

const opReview = "review.decide"

type ReviewInput struct {
	Outcome          domain.ReviewOutcome     `json:"outcome"`
	Reason           string                   `json:"reason"`
	RemediationItems []domain.RemediationItem `json:"remediation_items,omitempty"`
}

func (s *Service) Review(ctx context.Context, caseID string, meta WriteMeta, input ReviewInput) (*domain.DisclosureCase, error) {
	if err := meta.Validate(true); err != nil {
		return nil, err
	}
	input.Reason = strings.TrimSpace(input.Reason)
	for i := range input.RemediationItems {
		input.RemediationItems[i].FindingID = strings.TrimSpace(input.RemediationItems[i].FindingID)
		input.RemediationItems[i].Instruction = strings.TrimSpace(input.RemediationItems[i].Instruction)
	}
	operation := opReview + ":" + string(input.Outcome) + ":" + domain.PayloadDigest(input)
	if replay, ok, err := s.replay(ctx, operation, caseID, meta.RequestID); ok || err != nil {
		return replay, err
	}
	c, err := s.repo.Get(ctx, caseID)
	if err != nil {
		return nil, err
	}
	if c.Status != domain.StatusAwaitingReview {
		return nil, domain.InvalidState("仅待复核案件可以给出复核结论")
	}
	if c.Revision != meta.ExpectedRevision {
		return nil, domain.NewError(domain.CodeConflict, "案件 revision 已变化", "revision")
	}
	if c.ReviewerID != meta.ActorID {
		return nil, domain.Forbidden("仅指定的独立复核人可以给出结论")
	}
	id, err := s.newID("review_")
	if err != nil {
		return nil, err
	}
	review := domain.ReviewDecision{ID: id, CaseID: c.ID, ReviewerID: meta.ActorID, Outcome: input.Outcome,
		Reason: input.Reason, CaseRevision: c.Revision, CreatedAt: s.clock().UTC(),
		RemediationItems: append([]domain.RemediationItem(nil), input.RemediationItems...)}
	if err := review.Validate(c.SubmitterID); err != nil {
		return nil, err
	}
	from := c.Status
	c.Reviews = append(c.Reviews, review)
	var eventType string
	switch input.Outcome {
	case domain.ReviewReturned:
		targets := make(map[string]struct{}, len(input.RemediationItems))
		for _, item := range input.RemediationItems {
			if _, exists := targets[item.FindingID]; exists {
				return nil, domain.Invalid("remediation_items", "整改项发现标识不得重复")
			}
			known := false
			for _, finding := range c.Findings {
				if finding.ID == item.FindingID && finding.CaseID == c.ID {
					known = true
					break
				}
			}
			if !known {
				return nil, domain.Invalid("remediation_items", "整改项不属于当前复核版本")
			}
			targets[item.FindingID] = struct{}{}
		}
		for i := range c.Findings {
			_, targeted := targets[c.Findings[i].ID]
			if len(targets) == 0 || targeted {
				c.Findings[i].Reset()
			}
		}
		if err := c.Transition(domain.StatusChangesRequested, s.clock()); err != nil {
			return nil, err
		}
		eventType = "review.returned"
	case domain.ReviewApproved:
		if err := c.Transition(domain.StatusApproved, s.clock()); err != nil {
			return nil, err
		}
		eventType = "review.approved"
	default:
		return nil, domain.Invalid("outcome", "复核结论无效")
	}
	if c.RiskSummary != nil {
		var riskErr error
		c.RiskSummary, riskErr = redaction.BuildRiskSummaryForContent(c.ContentExcerpt, c.Findings, c.RiskSummary.DetectionRevision, c.Revision)
		if riskErr != nil {
			return nil, riskErr
		}
	}
	type remediationSummary struct {
		FindingID         string `json:"finding_id"`
		InstructionDigest string `json:"instruction_digest"`
	}
	summaries := make([]remediationSummary, 0, len(review.RemediationItems))
	for _, item := range review.RemediationItems {
		summaries = append(summaries, remediationSummary{item.FindingID, domain.DigestString(item.Instruction)})
	}
	event := s.event(c, eventType, meta, from, struct {
		ReviewID         string               `json:"review_id"`
		ReasonDigest     string               `json:"reason_digest"`
		RemediationItems []remediationSummary `json:"remediation_items,omitempty"`
	}{review.ID, domain.DigestString(review.Reason), summaries})
	if err := s.commit(ctx, operation, c, meta.ExpectedRevision, event, meta.RequestID, false); err != nil {
		return nil, err
	}
	return c.Clone()
}
