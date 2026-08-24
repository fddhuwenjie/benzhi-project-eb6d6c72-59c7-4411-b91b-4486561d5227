package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"archive-review/internal/domain"
	"archive-review/internal/workflow"
)

func (a *API) GetManifest(w http.ResponseWriter, r *http.Request) {
	caseID, err := requirePathValue(r, "caseID")
	if err != nil {
		handleError(w, err)
		return
	}
	manifest, err := a.workflow.Manifest(r.Context(), caseID)
	if err != nil {
		handleError(w, err)
		return
	}
	writeData(w, http.StatusOK, manifest)
}

func (a *API) PreviewManifest(w http.ResponseWriter, r *http.Request) {
	caseID, err := requirePathValue(r, "caseID")
	if err != nil {
		handleError(w, err)
		return
	}
	preview, err := a.workflow.PreviewManifest(r.Context(), caseID, strings.TrimSpace(r.Header.Get("X-Actor-ID")))
	if err != nil {
		handleError(w, err)
		return
	}
	writeData(w, http.StatusOK, preview)
}

func (a *API) GetRiskSummary(w http.ResponseWriter, r *http.Request) {
	caseID, err := requirePathValue(r, "caseID")
	if err != nil {
		handleError(w, err)
		return
	}
	summary, err := a.workflow.RiskSummary(r.Context(), caseID)
	if err != nil {
		handleError(w, err)
		return
	}
	writeData(w, http.StatusOK, summary)
}

func (a *API) GetReviewReadiness(w http.ResponseWriter, r *http.Request) {
	caseID, err := requirePathValue(r, "caseID")
	if err != nil {
		handleError(w, err)
		return
	}
	readiness, err := a.workflow.ReviewReadiness(r.Context(), caseID, r.Header.Get("X-Actor-ID"), r.URL.Query().Get("reviewer_id"))
	if err != nil {
		handleError(w, err)
		return
	}
	writeData(w, http.StatusOK, readiness)
}

func (a *API) GetWorkQueue(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	allowed := map[string]bool{"status": true, "source_department": true, "updated_from": true, "updated_to": true, "page_size": true, "cursor": true}
	for key, values := range query {
		if !allowed[key] {
			handleError(w, domain.Invalid(key, "未知的工作队列筛选字段"))
			return
		}
		if len(values) != 1 {
			handleError(w, domain.Invalid(key, "筛选字段不得重复"))
			return
		}
	}
	filter := workflow.QueueFilter{Status: domain.CaseStatus(strings.TrimSpace(query.Get("status"))),
		SourceDepartment: strings.TrimSpace(query.Get("source_department")), Cursor: strings.TrimSpace(query.Get("cursor"))}
	if value := strings.TrimSpace(query.Get("page_size")); value != "" {
		size, err := strconv.Atoi(value)
		if err != nil {
			handleError(w, domain.Invalid("page_size", "分页大小必须是整数"))
			return
		}
		filter.PageSize = size
	}
	var err error
	if filter.UpdatedFrom, err = parseOptionalTime(query.Get("updated_from"), "updated_from"); err != nil {
		handleError(w, err)
		return
	}
	if filter.UpdatedTo, err = parseOptionalTime(query.Get("updated_to"), "updated_to"); err != nil {
		handleError(w, err)
		return
	}
	result, err := a.workflow.WorkQueue(r.Context(), r.Header.Get("X-Actor-ID"), filter)
	if err != nil {
		handleError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func parseOptionalTime(value, field string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, domain.Invalid(field, "更新时间必须采用 RFC3339 格式")
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

func (a *API) GetTimeline(w http.ResponseWriter, r *http.Request) {
	caseID, err := requirePathValue(r, "caseID")
	if err != nil {
		handleError(w, err)
		return
	}
	entries, err := a.workflow.Timeline(r.Context(), caseID)
	if err != nil {
		handleError(w, err)
		return
	}
	writeData(w, http.StatusOK, entries)
}

func (a *API) GetAuditEvents(w http.ResponseWriter, r *http.Request) {
	caseID, err := requirePathValue(r, "caseID")
	if err != nil {
		handleError(w, err)
		return
	}
	events, err := a.workflow.AuditEvents(r.Context(), caseID)
	if err != nil {
		handleError(w, err)
		return
	}
	writeData(w, http.StatusOK, events)
}

func (a *API) GetAuditIntegrity(w http.ResponseWriter, r *http.Request) {
	caseID, err := requirePathValue(r, "caseID")
	if err != nil {
		handleError(w, err)
		return
	}
	result, err := a.workflow.AuditIntegrity(r.Context(), caseID)
	if err != nil {
		handleError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}
