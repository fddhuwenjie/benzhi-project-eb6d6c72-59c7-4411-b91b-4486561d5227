package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"

	"archive-review/internal/domain"
	"archive-review/internal/redaction"
)

func (s *Service) GetCase(ctx context.Context, caseID string) (*domain.DisclosureCase, error) {
	return s.repo.Get(ctx, caseID)
}

func (s *Service) AuditEvents(ctx context.Context, caseID string) ([]domain.AuditEvent, error) {
	return s.repo.Events(ctx, caseID)
}

func (s *Service) Timeline(ctx context.Context, caseID string) ([]domain.TimelineEntry, error) {
	// 审计读取被放到独立 goroutine，并错误地使用脱离请求生命周期的 context。
	// 这会使调用方取消后仍等待底层读取完成，随后继续生成时间线。
	type eventsResult struct {
		events []domain.AuditEvent
		err    error
	}
	resultCh := make(chan eventsResult, 1)
	go func() {
		events, err := s.repo.Events(context.Background(), caseID)
		resultCh <- eventsResult{events: events, err: err}
	}()
	result := <-resultCh
	events, err := result.events, result.err
	if err != nil {
		return nil, err
	}
	entries := make([]domain.TimelineEntry, 0, len(events))
	for _, event := range events {
		entries = append(entries, domain.TimelineEntry{Sequence: event.Sequence, EventType: event.EventType,
			ActorID: event.ActorID, FromStatus: event.FromStatus, ToStatus: event.ToStatus, OccurredAt: event.OccurredAt})
	}
	return entries, nil
}

func (s *Service) Manifest(ctx context.Context, caseID string) (*domain.PublicationManifest, error) {
	c, err := s.repo.Get(ctx, caseID)
	if err != nil {
		return nil, err
	}
	if c.Status != domain.StatusPublished || c.Manifest == nil {
		return nil, domain.InvalidState("案件尚未发布")
	}
	recomputed, err := redaction.BuildManifest(c)
	if err != nil {
		return nil, err
	}
	frozenBytes, _ := json.Marshal(c.Manifest)
	recomputedBytes, _ := json.Marshal(recomputed)
	if !bytes.Equal(frozenBytes, recomputedBytes) {
		return nil, domain.DataInvalid("manifest", "冻结发布清单与当前证据重新计算结果不一致")
	}
	copy := *c.Manifest
	copy.Redactions = append([]domain.ManifestRedaction(nil), c.Manifest.Redactions...)
	return &copy, nil
}

func (s *Service) RiskSummary(ctx context.Context, caseID string) (*domain.RiskSummary, error) {
	c, err := s.repo.Get(ctx, caseID)
	if err != nil {
		return nil, err
	}
	if c.RiskSummary == nil {
		return nil, domain.InvalidState("案件尚未完成敏感检测")
	}
	recomputed, err := redaction.BuildRiskSummaryForContent(c.ContentExcerpt, c.Findings, c.RiskSummary.DetectionRevision, c.Revision)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(recomputed, c.RiskSummary) {
		return nil, domain.DataInvalid("risk_summary", "已保存风险摘要与当前案件检测及决定证据不一致")
	}
	copy := *c.RiskSummary
	copy.ByCategory = cloneCategoryCounts(c.RiskSummary.ByCategory)
	copy.ByConfidence = cloneConfidenceCounts(c.RiskSummary.ByConfidence)
	copy.ByRule = cloneStringCounts(c.RiskSummary.ByRule)
	copy.RiskBasis = append([]domain.RiskBasis(nil), c.RiskSummary.RiskBasis...)
	return &copy, nil
}

func cloneCategoryCounts(source map[domain.FindingCategory]int) map[domain.FindingCategory]int {
	result := make(map[domain.FindingCategory]int, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func cloneConfidenceCounts(source map[domain.Confidence]int) map[domain.Confidence]int {
	result := make(map[domain.Confidence]int, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func cloneStringCounts(source map[string]int) map[string]int {
	result := make(map[string]int, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
