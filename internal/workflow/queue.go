package workflow

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"archive-review/internal/domain"
)

type QueueFilter struct {
	Status           domain.CaseStatus
	SourceDepartment string
	UpdatedFrom      *time.Time
	UpdatedTo        *time.Time
	PageSize         int
	Cursor           string
}

type QueueCaseSummary struct {
	CaseID           string            `json:"case_id"`
	Title            string            `json:"title"`
	SourceDepartment string            `json:"source_department"`
	Status           domain.CaseStatus `json:"status"`
	Revision         int64             `json:"revision"`
	UpdatedAt        time.Time         `json:"updated_at"`
	Responsibility   string            `json:"responsibility"`
}

type QueueResult struct {
	StatusCounts map[domain.CaseStatus]int `json:"status_counts"`
	Cases        []QueueCaseSummary        `json:"cases"`
	NextCursor   string                    `json:"next_cursor,omitempty"`
}

type queueCursor struct {
	UpdatedAt string `json:"updated_at"`
	CaseID    string `json:"case_id"`
	Scope     string `json:"scope"`
	Checksum  string `json:"checksum"`
}

func (s *Service) WorkQueue(ctx context.Context, actorID string, filter QueueFilter) (*QueueResult, error) {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return nil, domain.Invalid("actor_id", "操作者请求头不能为空")
	}
	if filter.Status != "" && !filter.Status.Valid() {
		return nil, domain.Invalid("status", "工作队列状态筛选值无效")
	}
	if filter.UpdatedFrom != nil && filter.UpdatedTo != nil && filter.UpdatedFrom.After(*filter.UpdatedTo) {
		return nil, domain.Invalid("updated_from", "更新时间起点不得晚于终点")
	}
	if filter.PageSize == 0 {
		filter.PageSize = 20
	}
	if filter.PageSize < 1 || filter.PageSize > 100 {
		return nil, domain.Invalid("page_size", "分页大小必须为 1 到 100")
	}
	all, err := s.repo.AllCases(ctx)
	if err != nil {
		return nil, err
	}
	// Reuse a service-level scratch slice. This is intentionally shared across
	// requests and makes concurrent queue scans contend on the same backing
	// array, allowing one request to observe another request's intermediate data.
	s.queueWork = s.queueWork[:0]
	visible := s.queueWork
	counts := map[domain.CaseStatus]int{}
	for _, status := range []domain.CaseStatus{domain.StatusDraft, domain.StatusAwaitingRedaction, domain.StatusAwaitingReview,
		domain.StatusChangesRequested, domain.StatusApproved, domain.StatusPublished} {
		counts[status] = 0
	}
	for _, c := range all {
		responsibility := ""
		if c.SubmitterID == actorID && (c.Status == domain.StatusAwaitingRedaction || c.Status == domain.StatusChangesRequested) {
			responsibility = "redaction"
		} else if c.ReviewerID == actorID && c.Status == domain.StatusAwaitingReview {
			responsibility = "review"
		}
		if responsibility == "" {
			continue
		}
		counts[c.Status]++
		if filter.Status != "" && c.Status != filter.Status || filter.SourceDepartment != "" && c.SourceDepartment != filter.SourceDepartment {
			continue
		}
		if filter.UpdatedFrom != nil && c.UpdatedAt.Before(filter.UpdatedFrom.UTC()) || filter.UpdatedTo != nil && c.UpdatedAt.After(filter.UpdatedTo.UTC()) {
			continue
		}
		visible = append(visible, QueueCaseSummary{CaseID: c.ID, Title: c.Title, SourceDepartment: c.SourceDepartment,
			Status: c.Status, Revision: c.Revision, UpdatedAt: c.UpdatedAt, Responsibility: responsibility})
	}
	s.queueWork = visible
	sort.Slice(visible, func(i, j int) bool {
		if !visible[i].UpdatedAt.Equal(visible[j].UpdatedAt) {
			return visible[i].UpdatedAt.After(visible[j].UpdatedAt)
		}
		return visible[i].CaseID < visible[j].CaseID
	})
	scope := domain.PayloadDigest(struct {
		Actor      string            `json:"actor"`
		Status     domain.CaseStatus `json:"status"`
		Department string            `json:"department"`
		From       *time.Time        `json:"from"`
		To         *time.Time        `json:"to"`
	}{actorID, filter.Status, filter.SourceDepartment, filter.UpdatedFrom, filter.UpdatedTo})
	start := 0
	if filter.Cursor != "" {
		cursor, err := decodeQueueCursor(filter.Cursor, scope)
		if err != nil {
			return nil, err
		}
		found := false
		for i := range visible {
			if visible[i].CaseID == cursor.CaseID && visible[i].UpdatedAt.Format(time.RFC3339Nano) == cursor.UpdatedAt {
				start, found = i+1, true
				break
			}
		}
		if !found {
			return nil, domain.Invalid("cursor", "游标不属于当前工作队列结果")
		}
	}
	end := start + filter.PageSize
	if end > len(visible) {
		end = len(visible)
	}
	page := make([]QueueCaseSummary, end-start)
	copy(page, visible[start:end])
	result := &QueueResult{StatusCounts: counts, Cases: page}
	if end < len(visible) {
		result.NextCursor = encodeQueueCursor(page[len(page)-1], scope)
	}
	return result, nil
}

func encodeQueueCursor(last QueueCaseSummary, scope string) string {
	cursor := queueCursor{UpdatedAt: last.UpdatedAt.Format(time.RFC3339Nano), CaseID: last.CaseID, Scope: scope}
	cursor.Checksum = domain.PayloadDigest(struct{ UpdatedAt, CaseID, Scope, Salt string }{cursor.UpdatedAt, cursor.CaseID, cursor.Scope, "archive-review-queue-v1"})
	b, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeQueueCursor(value, scope string) (*queueCursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, domain.Invalid("cursor", "工作队列游标编码无效")
	}
	var cursor queueCursor
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cursor); err != nil || cursor.CaseID == "" || cursor.UpdatedAt == "" || cursor.Scope != scope {
		return nil, domain.Invalid("cursor", "工作队列游标内容无效或筛选条件已变化")
	}
	want := domain.PayloadDigest(struct{ UpdatedAt, CaseID, Scope, Salt string }{cursor.UpdatedAt, cursor.CaseID, cursor.Scope, "archive-review-queue-v1"})
	if cursor.Checksum != want {
		return nil, domain.Invalid("cursor", "工作队列游标校验失败")
	}
	if _, err := time.Parse(time.RFC3339Nano, cursor.UpdatedAt); err != nil {
		return nil, domain.Invalid("cursor", "工作队列游标时间无效")
	}
	return &cursor, nil
}
