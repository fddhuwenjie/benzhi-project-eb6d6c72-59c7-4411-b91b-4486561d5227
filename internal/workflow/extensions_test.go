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

func newExtensionService(t *testing.T) (*Service, *store.DiskStore) {
	t.Helper()
	repo, err := store.OpenDisk(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 24, 16, 0, 0, 0, time.UTC)
	sequence := 0
	service := NewWithDependencies(repo, redaction.NewDefaultDetector(), func() time.Time {
		now = now.Add(time.Second)
		return now
	}, func(prefix string) (string, error) {
		sequence++
		return fmt.Sprintf("%s%d", prefix, sequence), nil
	})
	return service, repo
}

func TestDuplicateAcceptanceConflictIsSideEffectFree(t *testing.T) {
	service, repo := newExtensionService(t)
	ctx := context.Background()
	input := CreateInput{Title: "重复材料测试", SourceDepartment: "档案管理部门",
		ContentExcerpt: "姓名：张三，联系电话 13800138000，该材料用于重复受理验证。"}
	first, err := service.CreateCase(ctx, WriteMeta{ActorID: "author-a", RequestID: "create-first"}, input)
	if err != nil {
		t.Fatal(err)
	}
	before, err := repo.Events(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.CreateCase(ctx, WriteMeta{ActorID: "author-b", RequestID: "create-conflict"}, input)
	if domain.ErrorCodeOf(err) != domain.CodeDuplicate {
		t.Fatalf("重复材料未返回冲突: %v", err)
	}
	if _, err := repo.LookupRequest(ctx, "create-conflict"); domain.ErrorCodeOf(err) != domain.CodeNotFound {
		t.Fatalf("冲突请求不应写入幂等结果: %v", err)
	}
	after, _ := repo.Events(ctx, first.ID)
	all, _ := repo.AllCases(ctx)
	if len(after) != len(before) || len(all) != 1 {
		t.Fatalf("冲突请求产生副作用: events=%d cases=%d", len(after), len(all))
	}
	input.SourceDepartment = "政务公开部门"
	input.AllowDuplicate = true
	input.DuplicateReason = "属于另一部门独立形成的法定开放事项"
	second, err := service.CreateCase(ctx, WriteMeta{ActorID: "author-b", RequestID: "create-allowed"}, input)
	if err != nil {
		t.Fatal(err)
	}
	if second.DuplicateAcceptance == nil || second.DuplicateAcceptance.RelatedCases[0].DuplicateKind != "cross_department" {
		t.Fatalf("跨部门重复证据缺失: %+v", second.DuplicateAcceptance)
	}
	replayed, err := service.CreateCase(ctx, WriteMeta{ActorID: "author-b", RequestID: "create-allowed"}, input)
	if err != nil || replayed.ID != second.ID || replayed.Revision != second.Revision {
		t.Fatalf("重复受理幂等重放失败: %+v %v", replayed, err)
	}
}

func TestBatchReadinessTargetedReturnPreviewPublishAndIntegrity(t *testing.T) {
	service, repo := newExtensionService(t)
	ctx := context.Background()
	created, err := service.CreateCase(ctx, WriteMeta{ActorID: "submitter", RequestID: "create"}, CreateInput{
		Title: "扩展流程档案", SourceDepartment: "市档案管理部门",
		ContentExcerpt: "姓名：李四，电话 13912345678，邮箱 lisi@example.gov.cn，档案编号 DA-2026-77。"})
	if err != nil {
		t.Fatal(err)
	}
	detected, err := service.Detect(ctx, created.ID, WriteMeta{ActorID: "submitter", RequestID: "detect", ExpectedRevision: created.Revision})
	if err != nil {
		t.Fatal(err)
	}
	badBatch := BatchDecisionInput{Decisions: []BatchFindingDecision{
		{FindingID: detected.Findings[0].ID, Decision: domain.DecisionAccept, Reason: "确认遮蔽"},
		{FindingID: "fnd_unknown", Decision: domain.DecisionReject, Reason: "规则误报"},
	}}
	if _, err := service.DecideFindingsBatch(ctx, created.ID, WriteMeta{ActorID: "submitter", RequestID: "batch-bad", ExpectedRevision: detected.Revision}, badBatch); domain.ErrorCodeOf(err) != domain.CodeInvalid {
		t.Fatalf("包含未知发现项的批次应失败: %v", err)
	}
	unchanged, _ := repo.Get(ctx, created.ID)
	events, _ := repo.Events(ctx, created.ID)
	if unchanged.Revision != detected.Revision || unchanged.Findings[0].IsDecided() || len(events) != 2 {
		t.Fatalf("失败批次产生了部分写入: revision=%d events=%d", unchanged.Revision, len(events))
	}
	decisions := make([]BatchFindingDecision, 0, len(detected.Findings))
	for _, finding := range detected.Findings {
		decisions = append(decisions, BatchFindingDecision{FindingID: finding.ID, Decision: domain.DecisionAccept, Reason: "依据规则确认遮蔽"})
	}
	decided, err := service.DecideFindingsBatch(ctx, created.ID, WriteMeta{ActorID: "submitter", RequestID: "batch-good", ExpectedRevision: detected.Revision}, BatchDecisionInput{Decisions: decisions})
	if err != nil {
		t.Fatal(err)
	}
	if decided.Revision != detected.Revision+1 || decided.RiskSummary.Decisions.Accepted != len(decided.Findings) {
		t.Fatalf("批量决定结果错误: revision=%d risk=%+v", decided.Revision, decided.RiskSummary)
	}
	ready, err := service.ReviewReadiness(ctx, created.ID, "submitter", "reviewer")
	if err != nil || !ready.Ready || ready.Evidence == nil {
		t.Fatalf("案件应已就绪: %+v %v", ready, err)
	}
	submitted, err := service.SubmitReview(ctx, created.ID, WriteMeta{ActorID: "submitter", RequestID: "submit-1", ExpectedRevision: decided.Revision},
		SubmitReviewInput{ReviewerID: "reviewer", PreflightRevision: ready.PreflightRevision})
	if err != nil {
		t.Fatal(err)
	}
	targetID := submitted.Findings[0].ID
	returned, err := service.Review(ctx, created.ID, WriteMeta{ActorID: "reviewer", RequestID: "return", ExpectedRevision: submitted.Revision}, ReviewInput{
		Outcome: domain.ReviewReturned, Reason: "指定项替换范围需要调整",
		RemediationItems: []domain.RemediationItem{{FindingID: targetID, Instruction: "改用统一遮蔽标记"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	pending := 0
	for _, finding := range returned.Findings {
		if !finding.IsDecided() {
			pending++
			if finding.ID != targetID {
				t.Fatalf("非目标发现项被重置: %s", finding.ID)
			}
		}
	}
	if pending != 1 {
		t.Fatalf("定向退回应仅重置一项: %d", pending)
	}
	redecided, err := service.DecideFindingsBatch(ctx, created.ID, WriteMeta{ActorID: "submitter", RequestID: "batch-remediate", ExpectedRevision: returned.Revision},
		BatchDecisionInput{Decisions: []BatchFindingDecision{{FindingID: targetID, Decision: domain.DecisionAdjust,
			Replacement: "[档案敏感信息]", Reason: "按定向整改意见调整"}}})
	if err != nil {
		t.Fatal(err)
	}
	ready, err = service.ReviewReadiness(ctx, created.ID, "submitter", "reviewer")
	if err != nil || !ready.Ready {
		t.Fatalf("整改后应重新就绪: %+v %v", ready, err)
	}
	resubmitted, err := service.SubmitReview(ctx, created.ID, WriteMeta{ActorID: "submitter", RequestID: "submit-2", ExpectedRevision: redecided.Revision},
		SubmitReviewInput{ReviewerID: "reviewer", PreflightRevision: ready.PreflightRevision})
	if err != nil {
		t.Fatal(err)
	}
	approved, err := service.Review(ctx, created.ID, WriteMeta{ActorID: "reviewer", RequestID: "approve", ExpectedRevision: resubmitted.Revision},
		ReviewInput{Outcome: domain.ReviewApproved, Reason: "复核确认可以发布"})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := service.PreviewManifest(ctx, created.ID, "submitter")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.PublishChecked(ctx, created.ID, WriteMeta{ActorID: "submitter", RequestID: "publish-bad", ExpectedRevision: approved.Revision},
		PublishInput{ContentFingerprint: "tampered", PreviewRevision: preview.PreviewRevision}); domain.ErrorCodeOf(err) != domain.CodeConflict {
		t.Fatalf("篡改指纹未被拒绝: %v", err)
	}
	published, err := service.PublishChecked(ctx, created.ID, WriteMeta{ActorID: "submitter", RequestID: "publish", ExpectedRevision: approved.Revision},
		PublishInput{ContentFingerprint: preview.ContentFingerprint, PreviewRevision: preview.PreviewRevision})
	if err != nil {
		t.Fatal(err)
	}
	if published.Manifest.ContentFingerprint != preview.ContentFingerprint || published.Manifest.ManifestDigest != preview.ManifestDigest {
		t.Fatalf("发布证据与预览不一致")
	}
	integrity, err := service.AuditIntegrity(ctx, created.ID)
	if err != nil || !integrity.Passed {
		t.Fatalf("完整流程证据校验未通过: %+v %v", integrity, err)
	}
}

func TestWorkQueueCursorVisibilityAndTamperRejection(t *testing.T) {
	service, _ := newExtensionService(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		created, err := service.CreateCase(ctx, WriteMeta{ActorID: "owner", RequestID: fmt.Sprintf("queue-create-%d", i)}, CreateInput{
			Title: fmt.Sprintf("队列档案 %d", i), SourceDepartment: "队列测试部门",
			ContentExcerpt: fmt.Sprintf("这是第 %d 份内容不同且长度足够的队列测试档案材料。", i)})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = service.Detect(ctx, created.ID, WriteMeta{ActorID: "owner", RequestID: fmt.Sprintf("queue-detect-%d", i), ExpectedRevision: created.Revision}); err != nil {
			t.Fatal(err)
		}
	}
	first, err := service.WorkQueue(ctx, "owner", QueueFilter{Status: domain.StatusAwaitingRedaction, SourceDepartment: "队列测试部门", PageSize: 1})
	if err != nil || len(first.Cases) != 1 || first.NextCursor == "" || first.StatusCounts[domain.StatusAwaitingRedaction] != 3 {
		t.Fatalf("队列首屏错误: %+v %v", first, err)
	}
	second, err := service.WorkQueue(ctx, "owner", QueueFilter{Status: domain.StatusAwaitingRedaction, SourceDepartment: "队列测试部门", PageSize: 1, Cursor: first.NextCursor})
	if err != nil || len(second.Cases) != 1 || second.Cases[0].CaseID == first.Cases[0].CaseID {
		t.Fatalf("队列翻页错误: %+v %v", second, err)
	}
	tampered := first.NextCursor[:len(first.NextCursor)-1] + "A"
	if _, err := service.WorkQueue(ctx, "owner", QueueFilter{Status: domain.StatusAwaitingRedaction, SourceDepartment: "队列测试部门", PageSize: 1, Cursor: tampered}); domain.ErrorCodeOf(err) != domain.CodeInvalid {
		t.Fatalf("篡改游标未被拒绝: %v", err)
	}
	other, err := service.WorkQueue(ctx, "unrelated", QueueFilter{PageSize: 10})
	if err != nil || len(other.Cases) != 0 {
		t.Fatalf("无关操作者看到了队列案件: %+v %v", other, err)
	}
}
