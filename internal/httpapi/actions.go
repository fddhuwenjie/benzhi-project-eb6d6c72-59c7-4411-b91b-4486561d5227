package httpapi

import (
	"fmt"
	"net/http"

	"archive-review/internal/domain"
	"archive-review/internal/workflow"
)

type emptyInput struct{}

func (a *API) DetectFindings(w http.ResponseWriter, r *http.Request) {
	caseID, meta, ok := actionContext(w, r)
	if !ok {
		return
	}
	var input emptyInput
	if err := decodeJSON(w, r, &input); err != nil {
		badRequest(w, err)
		return
	}
	c, err := a.workflow.Detect(r.Context(), caseID, meta)
	if err != nil {
		handleError(w, err)
		return
	}
	writeCase(w, c)
}

func (a *API) DecideFinding(w http.ResponseWriter, r *http.Request) {
	caseID, meta, ok := actionContext(w, r)
	if !ok {
		return
	}
	findingID, err := requirePathValue(r, "findingID")
	if err != nil {
		handleError(w, err)
		return
	}
	var input workflow.FindingDecisionInput
	if err := decodeJSON(w, r, &input); err != nil {
		badRequest(w, err)
		return
	}
	c, err := a.workflow.DecideFinding(r.Context(), caseID, findingID, meta, input)
	if err != nil {
		handleError(w, err)
		return
	}
	writeCase(w, c)
}

func (a *API) DecideFindingsBatch(w http.ResponseWriter, r *http.Request) {
	caseID, meta, ok := actionContext(w, r)
	if !ok {
		return
	}
	var input workflow.BatchDecisionInput
	if err := decodeJSON(w, r, &input); err != nil {
		badRequest(w, err)
		return
	}
	c, err := a.workflow.DecideFindingsBatch(r.Context(), caseID, meta, input)
	if err != nil {
		handleError(w, err)
		return
	}
	writeCase(w, c)
}

func (a *API) SubmitReview(w http.ResponseWriter, r *http.Request) {
	caseID, meta, ok := actionContext(w, r)
	if !ok {
		return
	}
	var input workflow.SubmitReviewInput
	if err := decodeJSON(w, r, &input); err != nil {
		badRequest(w, err)
		return
	}
	c, err := a.workflow.SubmitReview(r.Context(), caseID, meta, input)
	if err != nil {
		handleError(w, err)
		return
	}
	writeCase(w, c)
}

func (a *API) DecideReview(w http.ResponseWriter, r *http.Request) {
	caseID, meta, ok := actionContext(w, r)
	if !ok {
		return
	}
	var input workflow.ReviewInput
	if err := decodeJSON(w, r, &input); err != nil {
		badRequest(w, err)
		return
	}
	c, err := a.workflow.Review(r.Context(), caseID, meta, input)
	if err != nil {
		handleError(w, err)
		return
	}
	writeCase(w, c)
}

func (a *API) PublishCase(w http.ResponseWriter, r *http.Request) {
	caseID, meta, ok := actionContext(w, r)
	if !ok {
		return
	}
	var input workflow.PublishInput
	if err := decodeJSON(w, r, &input); err != nil {
		badRequest(w, err)
		return
	}
	c, err := a.workflow.PublishChecked(r.Context(), caseID, meta, input)
	if err != nil {
		handleError(w, err)
		return
	}
	writeCase(w, c)
}

func actionContext(w http.ResponseWriter, r *http.Request) (string, workflow.WriteMeta, bool) {
	caseID, err := requirePathValue(r, "caseID")
	if err != nil {
		handleError(w, err)
		return "", workflow.WriteMeta{}, false
	}
	meta, err := writeMeta(r, true)
	if err != nil {
		handleError(w, err)
		return "", workflow.WriteMeta{}, false
	}
	return caseID, meta, true
}

func writeCase(w http.ResponseWriter, c *domain.DisclosureCase) {
	w.Header().Set("ETag", revisionETag(c.Revision))
	writeData(w, http.StatusOK, c)
}

func revisionETag(revision int64) string { return fmt.Sprintf(`"%d"`, revision) }
