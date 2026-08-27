package manifest_preview_stale_cache_test

import (
	"context"
	"testing"
	"time"

	"archive-review/internal/domain"
	"archive-review/internal/redaction"
	"archive-review/internal/store"
	"archive-review/internal/workflow"
)

type mutableRepository struct {
	current *domain.DisclosureCase
}

func (r *mutableRepository) Get(context.Context, string) (*domain.DisclosureCase, error) {
	return r.current.Clone()
}

func (r *mutableRepository) Create(context.Context, store.Commit) error { return nil }
func (r *mutableRepository) Save(_ context.Context, commit store.Commit) error {
	if err := commit.Case.Validate(); err != nil {
		return err
	}
	current, err := commit.Case.Clone()
	if err != nil {
		return err
	}
	r.current = current
	return nil
}
func (r *mutableRepository) Events(context.Context, string) ([]domain.AuditEvent, error) {
	return nil, nil
}
func (r *mutableRepository) LookupRequest(context.Context, string) (*store.RequestRecord, error) {
	return nil, domain.NotFound("请求", "missing")
}
func (r *mutableRepository) FindByContentDigest(context.Context, string) ([]*domain.DisclosureCase, error) {
	return nil, nil
}
func (r *mutableRepository) AllCases(context.Context) ([]*domain.DisclosureCase, error) {
	return nil, nil
}

func TestManifestPreviewCacheRejectsPublishedCase(t *testing.T) {
	now := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	c, err := domain.NewCase("case_preview_cache", "发布预览缓存案件", "市档案管理部门",
		"这是一份不包含敏感发现项但长度足够的档案开放测试材料。", "submitter", now)
	if err != nil {
		t.Fatal(err)
	}
	c.Status = domain.StatusApproved
	c.Revision = 2
	c.UpdatedAt = now.Add(time.Minute)
	c.Reviews = []domain.ReviewDecision{{ID: "review_1", CaseID: c.ID, ReviewerID: "reviewer",
		Outcome: domain.ReviewApproved, Reason: "复核确认可以公开", CaseRevision: 1, CreatedAt: now.Add(time.Minute)}}
	c.RiskSummary, err = redaction.BuildRiskSummaryForContent(c.ContentExcerpt, c.Findings, 1, c.Revision)
	if err != nil {
		t.Fatal(err)
	}
	repo := &mutableRepository{current: c}
	service := workflow.New(repo, redaction.NewDefaultDetector())

	first, err := service.PreviewManifest(context.Background(), c.ID, "submitter")
	if err != nil || first.PreviewRevision != c.Revision {
		t.Fatalf("首次预览失败: preview=%+v err=%v", first, err)
	}
	published, err := service.PublishChecked(context.Background(), c.ID, workflow.WriteMeta{
		ActorID: "submitter", RequestID: "publish-after-preview", ExpectedRevision: c.Revision,
	}, workflow.PublishInput{ContentFingerprint: first.ContentFingerprint, PreviewRevision: first.PreviewRevision})
	if err != nil {
		t.Fatalf("发布失败: %v", err)
	}

	second, err := service.PreviewManifest(context.Background(), c.ID, "submitter")
	if domain.ErrorCodeOf(err) != domain.CodeState {
		t.Fatalf("TestManifestPreviewCacheRejectsPublishedCase: published revision %d 仍返回旧预览 %+v，err=%v",
			published.Revision, second, err)
	}
}
