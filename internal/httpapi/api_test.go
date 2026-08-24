package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"archive-review/internal/domain"
	"archive-review/internal/redaction"
	"archive-review/internal/store"
	"archive-review/internal/workflow"
)

func TestAPIValidationAndConflictMapping(t *testing.T) {
	repo, err := store.OpenDisk(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(New(workflow.New(repo, redaction.NewDefaultDetector())).Handler())
	defer server.Close()
	bad := send(t, server.URL+"/api/v1/cases", http.MethodPost, []byte(`{"title":"测试档案","source_department":"档案部门","content_excerpt":"这是一份足够长度的测试档案内容。","unknown":1}`), map[string]string{
		"Content-Type": "application/json; charset=utf-8", "X-Actor-ID": "actor", "X-Request-ID": "bad-create"})
	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown field status=%d", bad.StatusCode)
	}
	bad.Body.Close()
	payload, _ := json.Marshal(workflow.CreateInput{Title: "测试档案", SourceDepartment: "档案管理部门", ContentExcerpt: "姓名：赵六，手机号码为 13712345678，可供测试。"})
	createdResp := send(t, server.URL+"/api/v1/cases", http.MethodPost, payload, map[string]string{
		"Content-Type": "application/json", "X-Actor-ID": "actor", "X-Request-ID": "create"})
	if createdResp.StatusCode != http.StatusCreated {
		t.Fatalf("create status=%d", createdResp.StatusCode)
	}
	var envelope struct {
		Data domain.DisclosureCase `json:"data"`
	}
	if err := json.NewDecoder(createdResp.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	createdResp.Body.Close()
	if createdResp.Header.Get("ETag") != `"1"` {
		t.Fatalf("etag=%q", createdResp.Header.Get("ETag"))
	}
	detectURL := server.URL + "/api/v1/cases/" + envelope.Data.ID + "/detect"
	conflict := send(t, detectURL, http.MethodPost, []byte(`{}`), map[string]string{
		"Content-Type": "application/json", "X-Actor-ID": "actor", "X-Request-ID": "detect-bad", "If-Match": "99"})
	if conflict.StatusCode != http.StatusConflict {
		t.Fatalf("conflict status=%d", conflict.StatusCode)
	}
	conflict.Body.Close()
	missingHeader := send(t, detectURL, http.MethodPost, []byte(`{}`), map[string]string{"Content-Type": "application/json"})
	if missingHeader.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing header status=%d", missingHeader.StatusCode)
	}
	missingHeader.Body.Close()
}

func send(t *testing.T, url, method string, body []byte, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}
