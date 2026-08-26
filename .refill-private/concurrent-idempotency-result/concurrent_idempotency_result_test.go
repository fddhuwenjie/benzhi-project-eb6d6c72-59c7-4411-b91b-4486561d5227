package concurrent_idempotency_result_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"archive-review/internal/domain"
	"archive-review/internal/redaction"
	"archive-review/internal/store"
	"archive-review/internal/workflow"
)

type barrier struct {
	want    int
	mu      sync.Mutex
	arrived int
	ready   chan struct{}
}

func newBarrier(want int) *barrier {
	return &barrier{want: want, ready: make(chan struct{})}
}

func (b *barrier) wait() {
	b.mu.Lock()
	b.arrived++
	if b.arrived == b.want {
		close(b.ready)
	}
	ready := b.ready
	b.mu.Unlock()
	<-ready
}

type synchronizedRepo struct {
	store.Repository
	requestID  string
	caseID     string
	lookups    *barrier
	gets       *barrier
	firstSaved chan struct{}
}

func (r *synchronizedRepo) LookupRequest(ctx context.Context, requestID string) (*store.RequestRecord, error) {
	record, err := r.Repository.LookupRequest(ctx, requestID)
	if requestID == r.requestID {
		r.lookups.wait()
	}
	return record, err
}

func (r *synchronizedRepo) Get(ctx context.Context, caseID string) (*domain.DisclosureCase, error) {
	c, err := r.Repository.Get(ctx, caseID)
	if caseID == r.caseID {
		r.gets.wait()
	}
	return c, err
}

func (r *synchronizedRepo) Save(ctx context.Context, commit store.Commit) error {
	if commit.Request == nil || commit.Request.RequestID != r.requestID {
		return r.Repository.Save(ctx, commit)
	}
	reviewID := commit.Case.Reviews[len(commit.Case.Reviews)-1].ID
	if reviewID == "review_1" {
		err := r.Repository.Save(ctx, commit)
		close(r.firstSaved)
		return err
	}
	<-r.firstSaved
	return r.Repository.Save(ctx, commit)
}

type reviewResult struct {
	caseValue *domain.DisclosureCase
	err       error
}

func TestConcurrentIdempotencyReturnsCanonicalResult(t *testing.T) {
	ctx := context.Background()
	repo, err := store.OpenDisk(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)
	setupIDs := int64(0)
	setup := workflow.NewWithDependencies(repo, redaction.NewDefaultDetector(), func() time.Time { return now },
		func(prefix string) (string, error) {
			setupIDs++
			return fmt.Sprintf("%s%d", prefix, setupIDs), nil
		})
	c, err := setup.CreateCase(ctx, workflow.WriteMeta{ActorID: "submitter", RequestID: "create"}, workflow.CreateInput{
		Title: "并发幂等复核", SourceDepartment: "档案管理部门", ContentExcerpt: "本材料记录公共会议议程与开放事项，可以依法公开查阅。",
	})
	if err != nil {
		t.Fatal(err)
	}
	c, err = setup.Detect(ctx, c.ID, workflow.WriteMeta{ActorID: "submitter", RequestID: "detect", ExpectedRevision: c.Revision})
	if err != nil {
		t.Fatal(err)
	}
	c, err = setup.SubmitReview(ctx, c.ID, workflow.WriteMeta{ActorID: "submitter", RequestID: "submit", ExpectedRevision: c.Revision},
		workflow.SubmitReviewInput{ReviewerID: "reviewer"})
	if err != nil {
		t.Fatal(err)
	}

	const requestID = "approve-concurrent"
	coordinated := &synchronizedRepo{Repository: repo, requestID: requestID, caseID: c.ID, lookups: newBarrier(2),
		gets: newBarrier(2), firstSaved: make(chan struct{})}
	var generated atomic.Int64
	service := workflow.NewWithDependencies(coordinated, redaction.NewDefaultDetector(), func() time.Time { return now.Add(time.Minute) },
		func(prefix string) (string, error) {
			return fmt.Sprintf("%s%d", prefix, generated.Add(1)), nil
		})
	results := make(chan reviewResult, 2)
	for i := 0; i < 2; i++ {
		go func() {
			result, reviewErr := service.Review(ctx, c.ID, workflow.WriteMeta{
				ActorID: "reviewer", RequestID: requestID, ExpectedRevision: c.Revision,
			}, workflow.ReviewInput{Outcome: domain.ReviewApproved, Reason: "复核通过"})
			results <- reviewResult{caseValue: result, err: reviewErr}
		}()
	}
	first, second := <-results, <-results
	if first.err != nil || second.err != nil {
		t.Fatalf("TestConcurrentIdempotencyReturnsCanonicalResult: 同一幂等请求应返回两次成功，实际错误为 %v 和 %v", first.err, second.err)
	}
	firstID := first.caseValue.Reviews[len(first.caseValue.Reviews)-1].ID
	secondID := second.caseValue.Reviews[len(second.caseValue.Reviews)-1].ID
	if firstID != secondID {
		persisted, getErr := repo.Get(ctx, c.ID)
		if getErr != nil {
			t.Fatal(getErr)
		}
		persistedID := persisted.Reviews[len(persisted.Reviews)-1].ID
		t.Fatalf("TestConcurrentIdempotencyReturnsCanonicalResult: 两次成功响应不一致，review IDs 为 %q 和 %q，持久化值为 %q", firstID, secondID, persistedID)
	}
}
