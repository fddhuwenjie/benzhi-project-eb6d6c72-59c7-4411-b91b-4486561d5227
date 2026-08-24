package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"archive-review/internal/domain"
	"archive-review/internal/workflow"
)

func runSelfTest(cfg config) error {
	root, err := os.MkdirTemp("", "archive-review-selftest-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	server, err := buildServer(filepath.Join(root, "data"))
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("自检监听 %s: %w", cfg.Addr, err)
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
		_ = listener.Close()
		<-serveErr
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client := newSmokeClient(listener.Addr().String())
	if err := waitHealthy(ctx, client); err != nil {
		return err
	}
	return executeSmokeFlow(ctx, client)
}

func waitHealthy(ctx context.Context, client *smokeClient) error {
	var last error
	for i := 0; i < 20; i++ {
		_, err := doJSON[map[string]string](client, ctx, http.MethodGet, "/healthz", nil, "", "", 0)
		if err == nil {
			return nil
		}
		last = err
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
	return fmt.Errorf("健康检查未就绪: %w", last)
}

func executeSmokeFlow(ctx context.Context, client *smokeClient) error {
	submitter, reviewer := "reviewer-submit-01", "reviewer-independent-02"
	created, err := doJSON[domain.DisclosureCase](client, ctx, http.MethodPost, "/api/v1/cases",
		workflow.CreateInput{Title: "某公共事项办理档案", SourceDepartment: "市公共档案管理处",
			ContentExcerpt: "姓名：张三，联系电话 13800138000，邮箱 zhangsan@example.gov.cn，档案编号 DA-2026-0001。该材料记录公共事项办理结果。"}, submitter, "smoke-create", 0)
	if err != nil {
		return err
	}
	if created.Status != domain.StatusDraft || created.Revision != 1 {
		return fmt.Errorf("创建结果状态或 revision 不正确")
	}
	detected, err := doJSON[domain.DisclosureCase](client, ctx, http.MethodPost, casePath(created.ID, "/detect"), struct{}{}, submitter, "smoke-detect", created.Revision)
	if err != nil {
		return err
	}
	if len(detected.Findings) < 3 {
		return fmt.Errorf("预期至少检测 3 个敏感片段，实际 %d", len(detected.Findings))
	}
	current := detected
	for i, finding := range detected.Findings {
		input := workflow.FindingDecisionInput{Decision: domain.DecisionAccept, Reason: "依据开放审核规范确认遮蔽"}
		current, err = doJSON[domain.DisclosureCase](client, ctx, http.MethodPatch,
			casePath(created.ID, "/findings/"+finding.ID), input, submitter, fmt.Sprintf("smoke-decision-%d", i), current.Revision)
		if err != nil {
			return err
		}
	}
	submitted, err := doJSON[domain.DisclosureCase](client, ctx, http.MethodPost, casePath(created.ID, "/review-submissions"),
		workflow.SubmitReviewInput{ReviewerID: reviewer}, submitter, "smoke-submit-1", current.Revision)
	if err != nil {
		return err
	}
	replayed, err := doJSON[domain.DisclosureCase](client, ctx, http.MethodPost, casePath(created.ID, "/review-submissions"),
		workflow.SubmitReviewInput{ReviewerID: reviewer}, submitter, "smoke-submit-1", current.Revision)
	if err != nil {
		return fmt.Errorf("幂等重放失败: %w", err)
	}
	if replayed.Revision != submitted.Revision || replayed.Status != submitted.Status {
		return fmt.Errorf("幂等重放结果不一致")
	}
	returned, err := doJSON[domain.DisclosureCase](client, ctx, http.MethodPost, casePath(created.ID, "/review-decisions"),
		workflow.ReviewInput{Outcome: domain.ReviewReturned, Reason: "需要采用统一替换标记后再次提交"}, reviewer, "smoke-return", submitted.Revision)
	if err != nil {
		return err
	}
	if returned.Status != domain.StatusChangesRequested {
		return fmt.Errorf("退回后状态不正确")
	}
	current = returned
	for i, finding := range returned.Findings {
		input := workflow.FindingDecisionInput{Decision: domain.DecisionAdjust, Replacement: "[档案敏感信息]", Reason: "按复核意见统一调整"}
		current, err = doJSON[domain.DisclosureCase](client, ctx, http.MethodPatch,
			casePath(created.ID, "/findings/"+finding.ID), input, submitter, fmt.Sprintf("smoke-redecision-%d", i), current.Revision)
		if err != nil {
			return err
		}
	}
	resubmitted, err := doJSON[domain.DisclosureCase](client, ctx, http.MethodPost, casePath(created.ID, "/review-submissions"),
		workflow.SubmitReviewInput{ReviewerID: reviewer}, submitter, "smoke-submit-2", current.Revision)
	if err != nil {
		return err
	}
	approved, err := doJSON[domain.DisclosureCase](client, ctx, http.MethodPost, casePath(created.ID, "/review-decisions"),
		workflow.ReviewInput{Outcome: domain.ReviewApproved, Reason: "复核确认所有敏感信息均已适当去标识"}, reviewer, "smoke-approve", resubmitted.Revision)
	if err != nil {
		return err
	}
	published, err := doJSON[domain.DisclosureCase](client, ctx, http.MethodPost, casePath(created.ID, "/publish"),
		struct{}{}, submitter, "smoke-publish", approved.Revision)
	if err != nil {
		return err
	}
	if published.Status != domain.StatusPublished || published.Manifest == nil || published.PublishedAt == nil {
		return fmt.Errorf("案件未完整进入已发布状态")
	}
	manifest, err := doJSON[domain.PublicationManifest](client, ctx, http.MethodGet, casePath(created.ID, "/manifest"), nil, "", "", 0)
	if err != nil {
		return err
	}
	if manifest.ManifestDigest != published.Manifest.ManifestDigest || manifest.ContentFingerprint == "" {
		return fmt.Errorf("发布清单核验失败")
	}
	timeline, err := doJSON[[]domain.TimelineEntry](client, ctx, http.MethodGet, casePath(created.ID, "/timeline"), nil, "", "", 0)
	if err != nil {
		return err
	}
	events, err := doJSON[[]domain.AuditEvent](client, ctx, http.MethodGet, casePath(created.ID, "/audit-events"), nil, "", "", 0)
	if err != nil {
		return err
	}
	if len(events) != len(timeline) || len(events) != 7+2*len(detected.Findings) {
		return fmt.Errorf("审计事件数量不完整: %d", len(events))
	}
	for i := 1; i < len(events); i++ {
		if events[i].Sequence != events[i-1].Sequence+1 {
			return fmt.Errorf("审计序列不连续")
		}
	}
	finalCase, err := doJSON[domain.DisclosureCase](client, ctx, http.MethodGet, casePath(created.ID, ""), nil, "", "", 0)
	if err != nil {
		return err
	}
	if finalCase.Status != domain.StatusPublished || finalCase.Revision != published.Revision {
		return fmt.Errorf("最终案件查询不一致")
	}
	return nil
}

func casePath(caseID, suffix string) string { return "/api/v1/cases/" + caseID + suffix }
