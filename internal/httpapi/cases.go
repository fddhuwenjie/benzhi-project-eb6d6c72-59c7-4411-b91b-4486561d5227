package httpapi

import (
	"net/http"

	"archive-review/internal/workflow"
)

func (a *API) Health(w http.ResponseWriter, _ *http.Request) {
	writeData(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) CreateCase(w http.ResponseWriter, r *http.Request) {
	meta, err := writeMeta(r, false)
	if err != nil {
		handleError(w, err)
		return
	}
	var input workflow.CreateInput
	if err := decodeJSON(w, r, &input); err != nil {
		badRequest(w, err)
		return
	}
	c, err := a.workflow.CreateCase(r.Context(), meta, input)
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("ETag", revisionETag(c.Revision))
	writeData(w, http.StatusCreated, c)
}

func (a *API) GetCase(w http.ResponseWriter, r *http.Request) {
	caseID, err := requirePathValue(r, "caseID")
	if err != nil {
		handleError(w, err)
		return
	}
	c, err := a.workflow.GetCase(r.Context(), caseID)
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("ETag", revisionETag(c.Revision))
	writeData(w, http.StatusOK, c)
}
