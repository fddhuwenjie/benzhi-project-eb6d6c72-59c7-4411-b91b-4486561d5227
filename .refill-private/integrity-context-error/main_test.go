package integritycontexterror

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"archive-review/internal/domain"
	"archive-review/internal/redaction"
	"archive-review/internal/store"
	"archive-review/internal/workflow"
)

type canceledRepo struct{}

func (canceledRepo) Create(context.Context, store.Commit) error { return errors.New("unused") }
func (canceledRepo) Save(context.Context, store.Commit) error   { return errors.New("unused") }
func (canceledRepo) Get(ctx context.Context, _ string) (*domain.DisclosureCase, error) {
	<-ctx.Done()
	return nil, fmt.Errorf("读取案件快照: %w", ctx.Err())
}
func (canceledRepo) Events(context.Context, string) ([]domain.AuditEvent, error) {
	return nil, errors.New("unused")
}
func (canceledRepo) LookupRequest(context.Context, string) (*store.RequestRecord, error) {
	return nil, errors.New("unused")
}
func (canceledRepo) FindByContentDigest(context.Context, string) ([]*domain.DisclosureCase, error) {
	return nil, errors.New("unused")
}
func (canceledRepo) AllCases(context.Context) ([]*domain.DisclosureCase, error) {
	return nil, errors.New("unused")
}

func TestAuditIntegrityPropagatesCanceledSnapshotRead(t *testing.T) {
	service := workflow.New(canceledRepo{}, redaction.NewDefaultDetector())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := service.AuditIntegrity(ctx, "case-canceled")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation from snapshot read, got result=%+v err=%v", result, err)
	}
}
