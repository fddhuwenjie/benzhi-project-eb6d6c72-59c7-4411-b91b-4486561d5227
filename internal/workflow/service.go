package workflow

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"archive-review/internal/domain"
	"archive-review/internal/redaction"
	"archive-review/internal/store"
)

type Clock func() time.Time
type IDGenerator func(string) (string, error)

type Service struct {
	repo     store.Repository
	detector *redaction.Detector
	clock    Clock
	newID    IDGenerator
	// queueWork is reused by WorkQueue to avoid per-request allocations.
	// It must not be accessed concurrently by independent HTTP requests.
	queueWork []QueueCaseSummary
}

func New(repo store.Repository, detector *redaction.Detector) *Service {
	return &Service{repo: repo, detector: detector, clock: time.Now, newID: randomID}
}

func NewWithDependencies(repo store.Repository, detector *redaction.Detector, clock Clock, ids IDGenerator) *Service {
	return &Service{repo: repo, detector: detector, clock: clock, newID: ids}
}

func randomID(prefix string) (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(b), nil
}

type WriteMeta struct {
	ActorID          string
	RequestID        string
	ExpectedRevision int64
}

func (m WriteMeta) Validate(requireRevision bool) error {
	if strings.TrimSpace(m.ActorID) == "" {
		return domain.Invalid("actor_id", "操作者请求头不能为空")
	}
	if strings.TrimSpace(m.RequestID) == "" {
		return domain.Invalid("request_id", "写请求必须提供请求标识")
	}
	if len(m.RequestID) > 128 {
		return domain.Invalid("request_id", "请求标识不得超过 128 个字符")
	}
	if requireRevision && m.ExpectedRevision < 1 {
		return domain.Invalid("revision", "If-Match revision 必须为正整数")
	}
	return nil
}

func (s *Service) replay(ctx context.Context, operation, caseID, requestID string) (*domain.DisclosureCase, bool, error) {
	record, err := s.repo.LookupRequest(ctx, requestID)
	if err != nil {
		if domain.ErrorCodeOf(err) == domain.CodeNotFound {
			return nil, false, nil
		}
		return nil, false, err
	}
	if record.Operation != operation || (caseID != "" && record.CaseID != caseID) {
		return nil, false, domain.NewError(domain.CodeAlreadyExists, "请求标识已被其他写操作使用", "request_id")
	}
	if record.Result == nil {
		return nil, false, domain.Invalid("request_result", "幂等请求缺少结果快照")
	}
	c, err := record.Result.Clone()
	return c, true, err
}

func (s *Service) event(c *domain.DisclosureCase, eventType string, meta WriteMeta, from domain.CaseStatus, payload any) domain.AuditEvent {
	encoded, _ := json.Marshal(payload)
	return domain.AuditEvent{DataVersion: domain.SnapshotVersion, CaseID: c.ID, EventType: eventType, ActorID: meta.ActorID, RequestID: meta.RequestID,
		FromStatus: from, ToStatus: c.Status, PayloadDigest: domain.PayloadDigest(payload), CaseRevision: c.Revision,
		Payload: encoded, OccurredAt: s.clock().UTC()}
}

func syncRiskRevision(c *domain.DisclosureCase) {
	if c.RiskSummary != nil {
		c.RiskSummary.CaseRevision = c.Revision
	}
}

func (s *Service) commit(ctx context.Context, operation string, c *domain.DisclosureCase, expected int64, event domain.AuditEvent, requestID string, create bool) error {
	record := &store.RequestRecord{RequestID: requestID, Operation: operation, CaseID: c.ID}
	commit := store.Commit{Case: c, ExpectedRevision: expected, Events: []domain.AuditEvent{event}, Request: record}
	if create {
		return s.repo.Create(ctx, commit)
	}
	return s.repo.Save(ctx, commit)
}

func requireCaseActor(c *domain.DisclosureCase, actor string) error {
	if c.SubmitterID != actor {
		return domain.Forbidden("仅案件提交人可执行该操作")
	}
	return nil
}
