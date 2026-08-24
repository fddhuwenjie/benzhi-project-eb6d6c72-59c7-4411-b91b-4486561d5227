package workflow

import (
	"context"

	"archive-review/internal/domain"
	"archive-review/internal/redaction"
)

const opPublish = "case.publish"

func (s *Service) Publish(ctx context.Context, caseID string, meta WriteMeta) (*domain.DisclosureCase, error) {
	return s.PublishChecked(ctx, caseID, meta, PublishInput{})
}

type PublishInput struct {
	ContentFingerprint string `json:"content_fingerprint,omitempty"`
	PreviewRevision    int64  `json:"preview_revision,omitempty"`
}

func (s *Service) PublishChecked(ctx context.Context, caseID string, meta WriteMeta, input PublishInput) (*domain.DisclosureCase, error) {
	if err := meta.Validate(true); err != nil {
		return nil, err
	}
	if input.PreviewRevision < 0 {
		return nil, domain.Invalid("preview_revision", "预览 revision 不得为负数")
	}
	operation := opPublish + ":" + domain.PayloadDigest(input)
	if replay, ok, err := s.replay(ctx, operation, caseID, meta.RequestID); ok || err != nil {
		return replay, err
	}
	c, err := s.repo.Get(ctx, caseID)
	if err != nil {
		return nil, err
	}
	if c.Status != domain.StatusApproved {
		return nil, domain.InvalidState("仅已批准案件可以发布")
	}
	if c.Revision != meta.ExpectedRevision {
		return nil, domain.NewError(domain.CodeConflict, "案件 revision 已变化", "revision")
	}
	if meta.ActorID != c.ReviewerID && meta.ActorID != c.SubmitterID {
		return nil, domain.Forbidden("仅案件提交人或批准复核人可以发布")
	}
	manifest, err := redaction.BuildManifest(c)
	if err != nil {
		return nil, err
	}
	if input.PreviewRevision != 0 && input.PreviewRevision != c.Revision {
		return nil, domain.NewError(domain.CodeConflict, "发布预览 revision 与当前案件不一致", "preview_revision")
	}
	if input.ContentFingerprint != "" && input.ContentFingerprint != manifest.ContentFingerprint {
		return nil, domain.NewError(domain.CodeConflict, "已核对内容指纹与当前候选发布内容不一致", "content_fingerprint")
	}
	from := c.Status
	c.Manifest = manifest
	if err := c.Transition(domain.StatusPublished, s.clock()); err != nil {
		return nil, err
	}
	syncRiskRevision(c)
	publishedAt := c.UpdatedAt
	c.PublishedAt = &publishedAt
	event := s.event(c, "case.published", meta, from, struct {
		ManifestDigest string `json:"manifest_digest"`
		Fingerprint    string `json:"content_fingerprint"`
	}{manifest.ManifestDigest, manifest.ContentFingerprint})
	if err := s.commit(ctx, operation, c, meta.ExpectedRevision, event, meta.RequestID, false); err != nil {
		return nil, err
	}
	return c.Clone()
}
