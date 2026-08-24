package workflow

import (
	"context"
	"strings"

	"archive-review/internal/domain"
	"archive-review/internal/redaction"
)

const opDecide = "finding.decide"

type FindingDecisionInput struct {
	Decision    domain.FindingDecision `json:"decision"`
	Replacement string                 `json:"replacement,omitempty"`
	Reason      string                 `json:"reason"`
}

func (s *Service) DecideFinding(ctx context.Context, caseID, findingID string, meta WriteMeta, input FindingDecisionInput) (*domain.DisclosureCase, error) {
	if err := meta.Validate(true); err != nil {
		return nil, err
	}
	input.Reason = strings.TrimSpace(input.Reason)
	operation := opDecide + ":" + findingID + ":" + domain.PayloadDigest(input)
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
		return nil, domain.InvalidState("当前状态不允许处理发现项")
	}
	if c.Revision != meta.ExpectedRevision {
		return nil, domain.NewError(domain.CodeConflict, "案件 revision 已变化", "revision")
	}
	index := -1
	for i := range c.Findings {
		if c.Findings[i].ID == findingID {
			index = i
			break
		}
	}
	if index < 0 {
		return nil, domain.NotFound("敏感发现项", findingID)
	}
	if err := c.Findings[index].Decide(input.Decision, input.Replacement, input.Reason, meta.ActorID, s.clock()); err != nil {
		return nil, err
	}
	from := c.Status
	c.Touch(s.clock())
	c.Findings[index].DecisionRevision = c.Revision
	detectionRevision := c.Revision
	if c.RiskSummary != nil {
		detectionRevision = c.RiskSummary.DetectionRevision
	}
	c.RiskSummary, err = redaction.BuildRiskSummaryForContent(c.ContentExcerpt, c.Findings, detectionRevision, c.Revision)
	if err != nil {
		return nil, err
	}
	event := s.event(c, "finding.decided", meta, from, struct {
		FindingID    string                 `json:"finding_id"`
		Decision     domain.FindingDecision `json:"decision"`
		ReasonDigest string                 `json:"reason_digest"`
	}{findingID, input.Decision, domain.DigestString(input.Reason)})
	if err := s.commit(ctx, operation, c, meta.ExpectedRevision, event, meta.RequestID, false); err != nil {
		return nil, err
	}
	return c.Clone()
}
