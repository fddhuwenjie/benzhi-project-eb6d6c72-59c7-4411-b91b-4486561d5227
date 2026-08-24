package store

import (
	"context"
	"testing"
	"time"

	"archive-review/internal/domain"
)

func TestDiskStoreConflictRecoveryAndIdempotency(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := OpenDisk(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	c, err := domain.NewCase("case_store", "测试档案", "档案管理部门", "这是一份用于持久化测试的公共档案材料。", "actor", now)
	if err != nil {
		t.Fatal(err)
	}
	created := domain.AuditEvent{CaseID: c.ID, EventType: "case.created", ActorID: "actor", RequestID: "req-create", ToStatus: c.Status, OccurredAt: now}
	if err := s.Create(ctx, Commit{Case: c, ExpectedRevision: 0, Events: []domain.AuditEvent{created},
		Request: &RequestRecord{RequestID: "req-create", Operation: "create", CaseID: c.ID}}); err != nil {
		t.Fatal(err)
	}
	updated, _ := c.Clone()
	if err := updated.Transition(domain.StatusAwaitingRedaction, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	updated.RiskSummary = &domain.RiskSummary{DetectionRevision: updated.Revision, CaseRevision: updated.Revision,
		FindingsDigest: domain.FindingsEvidenceDigest(updated.Findings), ByCategory: map[domain.FindingCategory]int{},
		ByConfidence: map[domain.Confidence]int{}, ByRule: map[string]int{}, RiskLevel: domain.RiskLow,
		RiskBasis: []domain.RiskBasis{{Code: "no_sensitive_findings", Message: "未检测到敏感发现项"}}}
	detected := domain.AuditEvent{CaseID: c.ID, EventType: "findings.detected", ActorID: "actor", RequestID: "req-detect",
		FromStatus: domain.StatusDraft, ToStatus: updated.Status, OccurredAt: now.Add(time.Minute)}
	if err := s.Save(ctx, Commit{Case: updated, ExpectedRevision: 99, Events: []domain.AuditEvent{detected},
		Request: &RequestRecord{RequestID: "req-bad", Operation: "detect", CaseID: c.ID}}); domain.ErrorCodeOf(err) != domain.CodeConflict {
		t.Fatalf("应发生冲突: %v", err)
	}
	events, err := s.Events(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("冲突写入了审计事件: %d", len(events))
	}
	if err := s.Save(ctx, Commit{Case: updated, ExpectedRevision: 1, Events: []domain.AuditEvent{detected},
		Request: &RequestRecord{RequestID: "req-detect", Operation: "detect", CaseID: c.ID}}); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenDisk(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.Get(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Revision != 2 || got.Status != domain.StatusAwaitingRedaction {
		t.Fatalf("恢复结果错误: %+v", got)
	}
	record, err := reopened.LookupRequest(ctx, "req-detect")
	if err != nil {
		t.Fatal(err)
	}
	if record.Result == nil || record.Result.Revision != 2 {
		t.Fatalf("幂等结果未恢复: %+v", record)
	}
	events, err = reopened.Events(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Sequence != 1 || events[1].Sequence != 2 {
		t.Fatalf("审计序列错误: %+v", events)
	}
}
