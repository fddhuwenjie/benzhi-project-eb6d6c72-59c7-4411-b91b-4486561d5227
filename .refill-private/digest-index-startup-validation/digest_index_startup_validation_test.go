package digest_index_startup_validation

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"archive-review/internal/redaction"
	"archive-review/internal/store"
	"archive-review/internal/workflow"
)

func TestOpenRejectsDigestIndexMissingExistingCase(t *testing.T) {
	dir := t.TempDir()
	repo, err := store.OpenDisk(dir)
	if err != nil {
		t.Fatal(err)
	}
	service := workflow.New(repo, redaction.NewDefaultDetector())
	_, err = service.CreateCase(context.Background(), workflow.WriteMeta{ActorID: "owner", RequestID: "create-1"}, workflow.CreateInput{
		Title: "索引恢复测试", SourceDepartment: "档案管理部门", ContentExcerpt: "这是一份用于摘要索引恢复校验的普通档案材料。",
	})
	if err != nil {
		t.Fatal(err)
	}
	broken, err := json.Marshal(struct {
		DataVersion int   `json:"data_version"`
		Entries     []any `json:"entries"`
	}{DataVersion: 1, Entries: []any{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "content-digests.json"), broken, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := store.OpenDisk(dir); err == nil {
		t.Fatal("重启时静默接受了缺失已有案件的内容摘要索引")
	}
}
