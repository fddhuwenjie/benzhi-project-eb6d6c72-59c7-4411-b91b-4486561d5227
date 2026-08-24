package redaction

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"archive-review/internal/domain"
)

func TestDetectRenderAndDeterministicManifest(t *testing.T) {
	content := "姓名：李四，电话 13912345678，邮箱 li.si@example.gov.cn，档案编号 AB-2026-99。"
	d := NewDefaultDetector()
	findings, err := d.Detect("case_redaction", content)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) < 4 {
		t.Fatalf("findings=%d: %+v", len(findings), findings)
	}
	for i := range findings {
		matched := content[findings[i].StartOffset:findings[i].EndOffset]
		if domain.DigestString(matched) != findings[i].MatchedTextDigest {
			t.Fatalf("偏移摘要不匹配: %q", matched)
		}
		if err := findings[i].Decide(domain.DecisionAccept, "", "确认敏感", "submitter", time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	rendered, err := Render(content, findings)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rendered, "13912345678") || strings.Contains(rendered, "example.gov.cn") {
		t.Fatalf("敏感内容仍存在: %s", rendered)
	}
	c, err := domain.NewCase("case_redaction", "测试档案", "档案管理部门", content, "submitter", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	c.Findings = findings
	c.Status = domain.StatusApproved
	c.ReviewerID = "reviewer"
	c.Reviews = []domain.ReviewDecision{{ID: "review_1", CaseID: c.ID, ReviewerID: "reviewer", Outcome: domain.ReviewApproved,
		Reason: "确认可以开放", CaseRevision: 4, CreatedAt: time.Now()}}
	m1, err := BuildManifest(c)
	if err != nil {
		t.Fatal(err)
	}
	m2, err := BuildManifest(c)
	if err != nil {
		t.Fatal(err)
	}
	b1, _ := json.Marshal(m1)
	b2, _ := json.Marshal(m2)
	if string(b1) != string(b2) {
		t.Fatalf("清单不确定\n%s\n%s", b1, b2)
	}
}

func TestMergeOverlappingRules(t *testing.T) {
	rules := []Rule{
		{ID: "low", Category: domain.CategoryContact, Confidence: domain.ConfidenceLow, Basis: "low", Pattern: mustPattern(`(12345)`)},
		{ID: "high", Category: domain.CategoryIdentity, Confidence: domain.ConfidenceHigh, Basis: "high", Pattern: mustPattern(`(34567)`)},
	}
	findings, err := NewDetector(rules).Detect("case_overlap", "xx1234567yy")
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("合并结果=%d", len(findings))
	}
	if findings[0].StartOffset != 2 || findings[0].EndOffset != 9 || findings[0].RuleID != "high" {
		t.Fatalf("合并区间错误: %+v", findings[0])
	}
}
