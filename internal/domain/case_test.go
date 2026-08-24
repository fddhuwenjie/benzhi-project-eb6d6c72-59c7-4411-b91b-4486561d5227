package domain

import (
	"testing"
	"time"
)

func TestCaseTransitionsAndFreeze(t *testing.T) {
	now := time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC)
	c, err := NewCase("case_test", "开放档案", "档案管理部门", "这是一份长度足够的公共档案材料内容。", "submitter", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Transition(StatusAwaitingReview, now); ErrorCodeOf(err) != CodeState {
		t.Fatalf("越级迁移应失败: %v", err)
	}
	if err := c.Transition(StatusAwaitingRedaction, now); err != nil {
		t.Fatal(err)
	}
	if c.Revision != 2 {
		t.Fatalf("revision=%d", c.Revision)
	}
	if err := c.Transition(StatusAwaitingReview, now); err != nil {
		t.Fatal(err)
	}
	if err := c.Transition(StatusApproved, now); err != nil {
		t.Fatal(err)
	}
	if err := c.Mutable(); ErrorCodeOf(err) != CodeState {
		t.Fatalf("批准后应冻结: %v", err)
	}
}

func TestFindingDecisionValidation(t *testing.T) {
	now := time.Now()
	f := SensitiveFinding{ID: "f1", CaseID: "c1", StartOffset: 0, EndOffset: 3, RuleID: "rule", MatchedTextDigest: "digest", Decision: DecisionPending}
	if err := f.Decide(DecisionAdjust, "", "需要调整", "actor", now); ErrorCodeOf(err) != CodeInvalid {
		t.Fatalf("空替换应失败: %v", err)
	}
	if err := f.Decide(DecisionReject, "", "规则误报", "actor", now); err != nil {
		t.Fatal(err)
	}
	if f.Decision != DecisionReject || f.DecidedAt == nil {
		t.Fatalf("决定未保存: %+v", f)
	}
	if err := f.Decide(DecisionAccept, "", "再次决定", "actor", now); ErrorCodeOf(err) != CodeState {
		t.Fatalf("重复决定应失败: %v", err)
	}
}
