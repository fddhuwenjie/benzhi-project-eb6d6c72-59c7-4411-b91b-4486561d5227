package workflow

import (
	"context"
	"strings"

	"archive-review/internal/domain"
	"archive-review/internal/redaction"
)

const opSubmitReview = "review.submit"

type SubmitReviewInput struct {
	ReviewerID        string `json:"reviewer_id"`
	PreflightRevision int64  `json:"preflight_revision,omitempty"`
}

func (s *Service) SubmitReview(ctx context.Context, caseID string, meta WriteMeta, input SubmitReviewInput) (*domain.DisclosureCase, error) {
	if err := meta.Validate(true); err != nil {
		return nil, err
	}
	input.ReviewerID = strings.TrimSpace(input.ReviewerID)
	operation := opSubmitReview + ":" + domain.PayloadDigest(input)
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
		return nil, domain.InvalidState("当前状态不能提交复核")
	}
	if c.Revision != meta.ExpectedRevision {
		return nil, domain.NewError(domain.CodeConflict, "案件 revision 已变化", "revision")
	}
	if input.PreflightRevision < 0 {
		return nil, domain.Invalid("preflight_revision", "预检 revision 不得为负数")
	}
	if input.PreflightRevision != 0 && input.PreflightRevision != c.Revision {
		return nil, domain.NewError(domain.CodeConflict, "案件在就绪预检后已发生变化，请重新预检", "preflight_revision")
	}
	reviewer := strings.TrimSpace(input.ReviewerID)
	if reviewer == "" {
		return nil, domain.Invalid("reviewer_id", "复核人不能为空")
	}
	if reviewer == c.SubmitterID {
		return nil, domain.Forbidden("提交人与复核人不得相同")
	}
	if !c.AllFindingsDecided() {
		return nil, domain.Invalid("findings", "所有敏感发现项完成决策后才能提交复核")
	}
	if _, err := redaction.Render(c.ContentExcerpt, c.Findings); err != nil {
		return nil, err
	}
	previousReviewID := ""
	if len(c.Reviews) > 0 && c.Reviews[len(c.Reviews)-1].Outcome == domain.ReviewReturned {
		previous := c.Reviews[len(c.Reviews)-1]
		previousReviewID = previous.ID
		for _, item := range previous.RemediationItems {
			found := false
			for _, finding := range c.Findings {
				if finding.ID == item.FindingID {
					found = finding.IsDecided() && finding.DecisionRevision > previous.CaseRevision
					break
				}
			}
			if !found {
				return nil, domain.Invalid("findings", "退回整改项必须产生晚于退回事件的新决定")
			}
		}
	}
	from := c.Status
	c.ReviewerID = reviewer
	if err := c.Transition(domain.StatusAwaitingReview, s.clock()); err != nil {
		return nil, err
	}
	syncRiskRevision(c)
	event := s.event(c, "review.submitted", meta, from, struct {
		ReviewerID       string `json:"reviewer_id"`
		PreviousReviewID string `json:"previous_review_id,omitempty"`
	}{reviewer, previousReviewID})
	if err := s.commit(ctx, operation, c, meta.ExpectedRevision, event, meta.RequestID, false); err != nil {
		return nil, err
	}
	return c.Clone()
}
