package partial_commit_state

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"archive-review/internal/domain"
	"archive-review/internal/redaction"
	"archive-review/internal/store"
	"archive-review/internal/workflow"
)

func TestCommitFailureLeavesNoPartialState(t *testing.T) {
	dir := t.TempDir()
	repo, err := store.OpenDisk(dir)
	if err != nil {
		t.Fatal(err)
	}
	service := workflow.New(repo, redaction.NewDefaultDetector())
	ctx := context.Background()
	created, err := service.CreateCase(ctx, workflow.WriteMeta{ActorID: "owner", RequestID: "create-1"}, workflow.CreateInput{
		Title: "提交原子性测试", SourceDepartment: "档案管理部门", ContentExcerpt: "这是一份用于提交失败回滚验证的普通档案材料。",
	})
	if err != nil {
		t.Fatal(err)
	}
	requestPath := filepath.Join(dir, "requests.json")
	if err := os.Remove(requestPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(requestPath, 0o750); err != nil {
		t.Fatal(err)
	}
	_, err = service.Detect(ctx, created.ID, workflow.WriteMeta{ActorID: "owner", RequestID: "detect-1", ExpectedRevision: created.Revision})
	if err == nil {
		t.Fatal("幂等索引持久化失败时应返回错误")
	}
	got, err := repo.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Revision != created.Revision || got.Status != domain.StatusDraft {
		t.Fatalf("幂等索引持久化失败后仍提交了案件快照: revision=%d status=%s", got.Revision, got.Status)
	}
	events, err := repo.Events(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("幂等索引持久化失败后仍追加了审计事件: %d", len(events))
	}
}
