package httpapi

import (
	"net/http"

	"archive-review/internal/workflow"
)

type API struct {
	workflow *workflow.Service
	mux      *http.ServeMux
}

func New(service *workflow.Service) *API {
	a := &API{workflow: service, mux: http.NewServeMux()}
	a.routes()
	return a
}

func (a *API) Handler() http.Handler {
	return securityHeaders(a.mux)
}

func (a *API) routes() {
	a.mux.HandleFunc("GET /healthz", a.Health)
	a.mux.HandleFunc("POST /api/v1/cases", a.CreateCase)
	a.mux.HandleFunc("GET /api/v1/cases/work-queue", a.GetWorkQueue)
	a.mux.HandleFunc("GET /api/v1/cases/{caseID}", a.GetCase)
	a.mux.HandleFunc("POST /api/v1/cases/{caseID}/detect", a.DetectFindings)
	a.mux.HandleFunc("PATCH /api/v1/cases/{caseID}/findings/{findingID}", a.DecideFinding)
	a.mux.HandleFunc("POST /api/v1/cases/{caseID}/findings/batch-decisions", a.DecideFindingsBatch)
	a.mux.HandleFunc("GET /api/v1/cases/{caseID}/risk-summary", a.GetRiskSummary)
	a.mux.HandleFunc("POST /api/v1/cases/{caseID}/review-submissions", a.SubmitReview)
	a.mux.HandleFunc("GET /api/v1/cases/{caseID}/review-submissions/readiness", a.GetReviewReadiness)
	a.mux.HandleFunc("POST /api/v1/cases/{caseID}/review-decisions", a.DecideReview)
	a.mux.HandleFunc("POST /api/v1/cases/{caseID}/publish", a.PublishCase)
	a.mux.HandleFunc("GET /api/v1/cases/{caseID}/manifest", a.GetManifest)
	a.mux.HandleFunc("GET /api/v1/cases/{caseID}/manifest/preview", a.PreviewManifest)
	a.mux.HandleFunc("GET /api/v1/cases/{caseID}/timeline", a.GetTimeline)
	a.mux.HandleFunc("GET /api/v1/cases/{caseID}/audit-events", a.GetAuditEvents)
	a.mux.HandleFunc("GET /api/v1/cases/{caseID}/audit-events/integrity", a.GetAuditIntegrity)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}
