package httpapi

import (
	"errors"
	"net/http"

	"archive-review/internal/domain"
)

func handleError(w http.ResponseWriter, err error) {
	var problem *domain.Error
	if errors.As(err, &problem) {
		status := http.StatusUnprocessableEntity
		switch problem.Code {
		case domain.CodeInvalid:
			status = http.StatusBadRequest
		case domain.CodeNotFound:
			status = http.StatusNotFound
		case domain.CodeConflict, domain.CodeAlreadyExists:
			status = http.StatusConflict
		case domain.CodeDuplicate:
			status = http.StatusConflict
		case domain.CodeData:
			status = http.StatusInternalServerError
		case domain.CodeForbidden:
			status = http.StatusForbidden
		case domain.CodeState:
			status = http.StatusUnprocessableEntity
		}
		writeDetailedError(w, status, string(problem.Code), problem.Message, problem.Field, problem.Details)
		return
	}
	writeError(w, http.StatusInternalServerError, "internal_error", "服务处理请求时发生内部错误", "")
}

func badRequest(w http.ResponseWriter, err error) {
	writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), "body")
}
