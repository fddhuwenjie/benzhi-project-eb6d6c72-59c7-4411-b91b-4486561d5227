package workflow

import (
	"context"

	"archive-review/internal/domain"
	"archive-review/internal/redaction"
)

const opDetect = "case.detect"

func (s *Service) Detect(ctx context.Context, caseID string, meta WriteMeta) (*domain.DisclosureCase, error) {
	if err := meta.Validate(true); err != nil {
		return nil, err
	}
	if replay, ok, err := s.replay(ctx, opDetect, caseID, meta.RequestID); ok || err != nil {
		return replay, err
	}
	c, err := s.repo.Get(ctx, caseID)
	if err != nil {
		return nil, err
	}
	if err := requireCaseActor(c, meta.ActorID); err != nil {
		return nil, err
	}
	if c.Status != domain.StatusDraft {
		return nil, domain.InvalidState("仅草稿案件可以执行敏感检测")
	}
	if c.Revision != meta.ExpectedRevision {
		return nil, domain.NewError(domain.CodeConflict, "案件 revision 已变化", "revision")
	}
	findings, err := s.detector.Detect(c.ID, c.ContentExcerpt)
	if err != nil {
		return nil, err
	}
	from := c.Status
	c.Findings = findings
	if err := c.Transition(domain.StatusAwaitingRedaction, s.clock()); err != nil {
		return nil, err
	}
	c.RiskSummary, err = redaction.BuildRiskSummaryForContent(c.ContentExcerpt, c.Findings, c.Revision, c.Revision)
	if err != nil {
		return nil, err
	}
	event := s.event(c, "findings.detected", meta, from, struct {
		Count int `json:"count"`
	}{len(findings)})
	if err := s.commit(ctx, opDetect, c, meta.ExpectedRevision, event, meta.RequestID, false); err != nil {
		return nil, err
	}
	return c.Clone()
}
