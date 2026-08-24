package store

import (
	"context"

	"archive-review/internal/domain"
)

type RequestRecord struct {
	RequestID string                 `json:"request_id"`
	Operation string                 `json:"operation"`
	CaseID    string                 `json:"case_id"`
	Revision  int64                  `json:"revision"`
	Result    *domain.DisclosureCase `json:"result"`
}

type Commit struct {
	Case             *domain.DisclosureCase
	ExpectedRevision int64
	Events           []domain.AuditEvent
	Request          *RequestRecord
}

type Repository interface {
	Create(context.Context, Commit) error
	Save(context.Context, Commit) error
	Get(context.Context, string) (*domain.DisclosureCase, error)
	Events(context.Context, string) ([]domain.AuditEvent, error)
	LookupRequest(context.Context, string) (*RequestRecord, error)
	FindByContentDigest(context.Context, string) ([]*domain.DisclosureCase, error)
	AllCases(context.Context) ([]*domain.DisclosureCase, error)
}
