package contextcancelcommit

import (
	"context"
	"testing"
	"time"

	"archive-review/internal/domain"
	"archive-review/internal/redaction"
	"archive-review/internal/store"
	"archive-review/internal/workflow"
)

type cancelAfterGetRepo struct {
	caseValue *domain.DisclosureCase
	cancel    context.CancelFunc
	saved     bool
}

func (r *cancelAfterGetRepo) Create(context.Context, store.Commit) error { return nil }

func (r *cancelAfterGetRepo) Save(ctx context.Context, commit store.Commit) error {
	r.saved = true
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (r *cancelAfterGetRepo) Get(context.Context, string) (*domain.DisclosureCase, error) {
	r.cancel()
	return r.caseValue.Clone()
}

func (r *cancelAfterGetRepo) Events(context.Context, string) ([]domain.AuditEvent, error) {
	return nil, domain.NotFound("审计事件", "case")
}

func (r *cancelAfterGetRepo) LookupRequest(context.Context, string) (*store.RequestRecord, error) {
	return nil, domain.NotFound("请求", "request")
}

func (r *cancelAfterGetRepo) FindByContentDigest(context.Context, string) ([]*domain.DisclosureCase, error) {
	return nil, nil
}

func (r *cancelAfterGetRepo) AllCases(context.Context) ([]*domain.DisclosureCase, error) {
	return nil, nil
}

func TestCanceledRequestDoesNotCommit(t *testing.T) {
	caseValue, err := domain.NewCase("case_ctx", "上下文案件", "档案管理部门", "这是一份用于取消传播测试的公共档案材料。", "submitter", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	repo := &cancelAfterGetRepo{caseValue: caseValue, cancel: cancel}
	service := workflow.New(repo, redaction.NewDefaultDetector())

	_, err = service.Detect(ctx, caseValue.ID, workflow.WriteMeta{
		ActorID: "submitter", RequestID: "detect-cancel", ExpectedRevision: caseValue.Revision,
	})
	if err != context.Canceled {
		t.Fatalf("取消的请求应返回 context.Canceled，实际为 %v", err)
	}
	if repo.saved {
		t.Fatal("请求取消后仍调用了持久化 Save")
	}
}
