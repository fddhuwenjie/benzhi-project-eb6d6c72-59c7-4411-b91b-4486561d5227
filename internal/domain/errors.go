package domain

import (
	"errors"
	"fmt"
)

type ErrorCode string

const (
	CodeInvalid       ErrorCode = "invalid_argument"
	CodeNotFound      ErrorCode = "not_found"
	CodeConflict      ErrorCode = "revision_conflict"
	CodeState         ErrorCode = "invalid_state"
	CodeForbidden     ErrorCode = "forbidden"
	CodeAlreadyExists ErrorCode = "already_exists"
	CodeDuplicate     ErrorCode = "duplicate_material_conflict"
	CodeData          ErrorCode = "data_validation_error"
)

type Error struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Field   string    `json:"field,omitempty"`
	Details any       `json:"details,omitempty"`
}

func (e *Error) Error() string {
	if e.Field == "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("%s: %s (%s)", e.Code, e.Message, e.Field)
}

func NewError(code ErrorCode, message, field string) error {
	return &Error{Code: code, Message: message, Field: field}
}

func NewDetailedError(code ErrorCode, message, field string, details any) error {
	return &Error{Code: code, Message: message, Field: field, Details: details}
}

func ErrorCodeOf(err error) ErrorCode {
	var target *Error
	if errors.As(err, &target) {
		return target.Code
	}
	return "internal_error"
}

func Invalid(field, message string) error     { return NewError(CodeInvalid, message, field) }
func InvalidState(message string) error       { return NewError(CodeState, message, "status") }
func Forbidden(message string) error          { return NewError(CodeForbidden, message, "actor_id") }
func DataInvalid(field, message string) error { return NewError(CodeData, message, field) }
func NotFound(kind, id string) error {
	return NewError(CodeNotFound, fmt.Sprintf("未找到%s %q", kind, id), "id")
}
