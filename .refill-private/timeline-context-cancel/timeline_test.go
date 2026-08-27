package timeline_context_cancel_test

import (
	"context"
	"errors"
	"testing"

	"archive-review/internal/domain"
	"archive-review/internal/redaction"
	"archive-review/internal/store"
	"archive-review/internal/workflow"
)

type blockingRepository struct {
	started chan<- struct{}
	release <-chan struct{}
}

func (r *blockingRepository) Create(context.Context, store.Commit) error { return nil }
func (r *blockingRepository) Save(context.Context, store.Commit) error   { return nil }
func (r *blockingRepository) Get(context.Context, string) (*domain.DisclosureCase, error) {
	return nil, domain.NotFound("案件", "case-timeline")
}
func (r *blockingRepository) Events(ctx context.Context, _ string) ([]domain.AuditEvent, error) {
	r.started <- struct{}{}
	<-r.release
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return []domain.AuditEvent{{Sequence: 1, CaseID: "case-timeline", EventType: "case.created"}}, nil
}
func (r *blockingRepository) LookupRequest(context.Context, string) (*store.RequestRecord, error) {
	return nil, domain.NotFound("请求", "request")
}
func (r *blockingRepository) FindByContentDigest(context.Context, string) ([]*domain.DisclosureCase, error) {
	return nil, nil
}
func (r *blockingRepository) AllCases(context.Context) ([]*domain.DisclosureCase, error) {
	return nil, nil
}

func TestTimelinePropagatesCancellationDuringAuditRead(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	repo := &blockingRepository{started: started, release: release}
	service := workflow.New(repo, redaction.NewDefaultDetector())
	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan error, 1)
	go func() {
		_, err := service.Timeline(ctx, "case-timeline")
		resultCh <- err
	}()
	<-started
	cancel()
	close(release)
	if err := <-resultCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("Timeline error = %v, want context.Canceled", err)
	}
}
