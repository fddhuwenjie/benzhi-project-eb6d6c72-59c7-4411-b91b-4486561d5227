package workflow

import (
	"context"
	"fmt"
	"testing"
	"time"

	"archive-review/internal/domain"
	"archive-review/internal/redaction"
	"archive-review/internal/store"
)

func TestCompleteWorkflowReturnApprovePublishAndFreeze(t *testing.T) {
	repo, err := store.OpenDisk(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 24, 14, 0, 0, 0, time.UTC)
	sequence := 0
	service := NewWithDependencies(repo, redaction.NewDefaultDetector(), func() time.Time { now = now.Add(time.Second); return now },
		func(prefix string) (string, error) { sequence++; return fmt.Sprintf("%s%d", prefix, sequence), nil })
	ctx := context.Background()
	c, err := service.CreateCase(ctx, WriteMeta{ActorID: "submitter", RequestID: "create"}, CreateInput{
		Title: "开放审核档案", SourceDepartment: "档案管理部门", ContentExcerpt: "姓名：王五，手机 13612345678，邮箱 wangwu@example.gov.cn，材料可依法开放。"})
	if err != nil {
		t.Fatal(err)
	}
	c, err = service.Detect(ctx, c.ID, WriteMeta{ActorID: "submitter", RequestID: "detect", ExpectedRevision: c.Revision})
	if err != nil {
		t.Fatal(err)
	}
	for i, finding := range c.Findings {
		c, err = service.DecideFinding(ctx, c.ID, finding.ID, WriteMeta{ActorID: "submitter", RequestID: fmt.Sprintf("d1-%d", i), ExpectedRevision: c.Revision},
			FindingDecisionInput{Decision: domain.DecisionAccept, Reason: "确认遮蔽"})
		if err != nil {
			t.Fatal(err)
		}
	}
	c, err = service.SubmitReview(ctx, c.ID, WriteMeta{ActorID: "submitter", RequestID: "submit1", ExpectedRevision: c.Revision}, SubmitReviewInput{ReviewerID: "reviewer"})
	if err != nil {
		t.Fatal(err)
	}
	staleRevision := c.Revision - 1
	replayed, err := service.SubmitReview(ctx, c.ID, WriteMeta{ActorID: "submitter", RequestID: "submit1", ExpectedRevision: staleRevision}, SubmitReviewInput{ReviewerID: "reviewer"})
	if err != nil || replayed.Revision != c.Revision {
		t.Fatalf("重放失败: %+v %v", replayed, err)
	}
	c, err = service.Review(ctx, c.ID, WriteMeta{ActorID: "reviewer", RequestID: "return", ExpectedRevision: c.Revision}, ReviewInput{Outcome: domain.ReviewReturned, Reason: "替换标记需统一"})
	if err != nil {
		t.Fatal(err)
	}
	for i, finding := range c.Findings {
		c, err = service.DecideFinding(ctx, c.ID, finding.ID, WriteMeta{ActorID: "submitter", RequestID: fmt.Sprintf("d2-%d", i), ExpectedRevision: c.Revision},
			FindingDecisionInput{Decision: domain.DecisionAdjust, Replacement: "[敏感信息]", Reason: "按意见调整"})
		if err != nil {
			t.Fatal(err)
		}
	}
	c, err = service.SubmitReview(ctx, c.ID, WriteMeta{ActorID: "submitter", RequestID: "submit2", ExpectedRevision: c.Revision}, SubmitReviewInput{ReviewerID: "reviewer"})
	if err != nil {
		t.Fatal(err)
	}
	c, err = service.Review(ctx, c.ID, WriteMeta{ActorID: "reviewer", RequestID: "approve", ExpectedRevision: c.Revision}, ReviewInput{Outcome: domain.ReviewApproved, Reason: "复核通过"})
	if err != nil {
		t.Fatal(err)
	}
	approvedRevision := c.Revision
	if _, err := service.DecideFinding(ctx, c.ID, c.Findings[0].ID, WriteMeta{ActorID: "submitter", RequestID: "late-edit", ExpectedRevision: c.Revision}, FindingDecisionInput{Decision: domain.DecisionReject, Reason: "尝试修改"}); domain.ErrorCodeOf(err) != domain.CodeState {
		t.Fatalf("批准后修改未被拒绝: %v", err)
	}
	c, err = service.Publish(ctx, c.ID, WriteMeta{ActorID: "submitter", RequestID: "publish", ExpectedRevision: approvedRevision})
	if err != nil {
		t.Fatal(err)
	}
	if c.Status != domain.StatusPublished || c.Manifest == nil {
		t.Fatalf("未发布: %+v", c)
	}
	events, err := service.AuditEvents(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if events[len(events)-1].EventType != "case.published" {
		t.Fatalf("最终审计事件错误: %+v", events[len(events)-1])
	}
}
