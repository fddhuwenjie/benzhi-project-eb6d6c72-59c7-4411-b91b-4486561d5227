package queue_shared_work_buffer

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"archive-review/internal/domain"
	"archive-review/internal/redaction"
	"archive-review/internal/store"
	"archive-review/internal/workflow"
)

type queueRepo struct {
	start chan struct{}
	cases []*domain.DisclosureCase
}

func (r *queueRepo) AllCases(context.Context) ([]*domain.DisclosureCase, error) {
	<-r.start
	return r.cases, nil
}
func (r *queueRepo) Create(context.Context, store.Commit) error { return nil }
func (r *queueRepo) Save(context.Context, store.Commit) error   { return nil }
func (r *queueRepo) Get(context.Context, string) (*domain.DisclosureCase, error) {
	return nil, domain.NotFound("案件", "missing")
}
func (r *queueRepo) Events(context.Context, string) ([]domain.AuditEvent, error) { return nil, nil }
func (r *queueRepo) LookupRequest(context.Context, string) (*store.RequestRecord, error) {
	return nil, domain.NotFound("请求", "missing")
}
func (r *queueRepo) FindByContentDigest(context.Context, string) ([]*domain.DisclosureCase, error) {
	return nil, nil
}

func TestWorkQueueConcurrentRequestsRace(t *testing.T) {
	runtime.GOMAXPROCS(2)
	now := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	cases := make([]*domain.DisclosureCase, 0, 4000)
	for i := 0; i < 4000; i++ {
		c, err := domain.NewCase(fmt.Sprintf("case-%04d", i), "并发队列案件", "档案管理部门", "这是一段用于队列并发扫描的材料内容。", "actor", now)
		if err != nil {
			t.Fatal(err)
		}
		c.Status = domain.StatusAwaitingRedaction
		cases = append(cases, c)
	}
	repo := &queueRepo{start: make(chan struct{}), cases: cases}
	service := workflow.New(repo, redaction.NewDefaultDetector())
	close(repo.start)

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := service.WorkQueue(context.Background(), "actor", workflow.QueueFilter{PageSize: 100})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("并发工作队列查询失败: %v", err)
		}
	}
}
