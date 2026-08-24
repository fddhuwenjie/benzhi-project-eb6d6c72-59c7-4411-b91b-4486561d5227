package workflow

import (
	"context"
	"strings"

	"archive-review/internal/domain"
	"archive-review/internal/redaction"
)

func (s *Service) PreviewManifest(ctx context.Context, caseID, actorID string) (*domain.ManifestPreview, error) {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return nil, domain.Invalid("actor_id", "操作者请求头不能为空")
	}
	c, err := s.repo.Get(ctx, caseID)
	if err != nil {
		return nil, err
	}
	if c.Status != domain.StatusApproved {
		return nil, domain.InvalidState("仅已批准案件可以预览发布清单")
	}
	approval := c.LatestApproval()
	if actorID != c.SubmitterID && (approval == nil || actorID != approval.ReviewerID) {
		return nil, domain.Forbidden("仅案件提交人或批准复核人可以预览发布清单")
	}
	manifest, err := redaction.BuildManifest(c)
	if err != nil {
		return nil, err
	}
	redactionCount := 0
	for _, item := range manifest.Redactions {
		if item.Decision == domain.DecisionAccept || item.Decision == domain.DecisionAdjust {
			redactionCount++
		}
	}
	return &domain.ManifestPreview{PublishedContent: manifest.PublishedContent, RedactionCount: redactionCount,
		ContentFingerprint: manifest.ContentFingerprint, ManifestDigest: manifest.ManifestDigest, PreviewRevision: c.Revision}, nil
}
