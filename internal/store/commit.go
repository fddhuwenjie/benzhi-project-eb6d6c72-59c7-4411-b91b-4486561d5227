package store

import (
	"context"
	"errors"
	"fmt"
	"os"

	"archive-review/internal/domain"
)

func (s *DiskStore) commit(ctx context.Context, commit Commit, creating bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if commit.Case == nil {
		return domain.Invalid("case", "提交案件不能为空")
	}
	if !safeID.MatchString(commit.Case.ID) {
		return domain.Invalid("id", "案件标识格式无效")
	}
	if err := commit.Case.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path := s.casePath(commit.Case.ID)
	current, readErr := s.readCaseUnlocked(path)
	if creating {
		if readErr == nil {
			return domain.NewError(domain.CodeAlreadyExists, "案件已经存在", "id")
		}
		if !errors.Is(readErr, os.ErrNotExist) {
			return fmt.Errorf("检查案件是否存在: %w", readErr)
		}
		if commit.ExpectedRevision != 0 {
			return domain.Invalid("expected_revision", "创建案件的期望修订号必须为 0")
		}
		indexed := s.digests[commit.Case.ContentDigest]
		if len(indexed) > 0 {
			if commit.Case.DuplicateAcceptance == nil {
				details := make([]domain.DuplicateCaseSummary, 0, len(indexed))
				for _, id := range indexed {
					existing, err := s.readCaseUnlocked(s.casePath(id))
					if err != nil {
						return domain.DataInvalid("content_digest_index", "内容摘要索引指向无法读取的案件")
					}
					kind := "cross_department"
					if existing.SourceDepartment == commit.Case.SourceDepartment {
						kind = "same_department"
					}
					details = append(details, domain.DuplicateCaseSummary{CaseID: id, Status: existing.Status,
						SourceDepartment: existing.SourceDepartment, DuplicateKind: kind})
				}
				return domain.NewDetailedError(domain.CodeDuplicate, "相同材料已在并发请求中受理，请重新确认重复受理", "allow_duplicate", details)
			}
			evidence := map[string]bool{}
			for _, related := range commit.Case.DuplicateAcceptance.RelatedCases {
				evidence[related.CaseID] = true
			}
			for _, id := range indexed {
				if !evidence[id] {
					return domain.NewError(domain.CodeConflict, "重复受理证据未覆盖当前内容摘要索引", "duplicate_acceptance")
				}
				existing, err := s.readCaseUnlocked(s.casePath(id))
				if err != nil {
					return domain.DataInvalid("content_digest_index", "内容摘要索引指向无法读取的案件")
				}
				for _, related := range commit.Case.DuplicateAcceptance.RelatedCases {
					if related.CaseID != id {
						continue
					}
					kind := "cross_department"
					if existing.SourceDepartment == commit.Case.SourceDepartment {
						kind = "same_department"
					}
					if related.Status != existing.Status || related.SourceDepartment != existing.SourceDepartment || related.DuplicateKind != kind {
						return domain.NewError(domain.CodeConflict, "重复受理证据与当前关联案件不一致，请重新确认", "duplicate_acceptance")
					}
				}
			}
		}
	} else {
		if errors.Is(readErr, os.ErrNotExist) {
			return domain.NotFound("案件", commit.Case.ID)
		}
		if readErr != nil {
			return fmt.Errorf("读取当前案件: %w", readErr)
		}
		if current.Revision != commit.ExpectedRevision {
			return domain.NewError(domain.CodeConflict, fmt.Sprintf("期望 revision %d，当前为 %d", commit.ExpectedRevision, current.Revision), "revision")
		}
		if commit.Case.ContentDigest != current.ContentDigest || commit.Case.ContentExcerpt != current.ContentExcerpt {
			return domain.DataInvalid("content_digest", "写操作不得改变已索引的源内容或摘要")
		}
		if commit.Case.Revision <= current.Revision {
			return domain.Invalid("revision", "新修订号必须递增")
		}
	}
	nextDigests := s.digests
	var digestTemp string
	if creating {
		nextDigests = cloneDigests(s.digests)
		for _, indexedID := range nextDigests[commit.Case.ContentDigest] {
			if indexedID == commit.Case.ID {
				return domain.DataInvalid("content_digest_index", "内容摘要索引已包含待创建案件")
			}
		}
		nextDigests[commit.Case.ContentDigest] = append(nextDigests[commit.Case.ContentDigest], commit.Case.ID)
		var err error
		digestTemp, err = s.prepareDigestIndex(nextDigests)
		if err != nil {
			return err
		}
		defer func() { _ = os.Remove(digestTemp) }()
	} else if err := s.validateDigestLink(current); err != nil {
		return err
	}
	if commit.Request != nil {
		if err := validateRequestRecord(*commit.Request, commit.Case); err != nil {
			return err
		}
		if existing, ok := s.requests[commit.Request.RequestID]; ok {
			if existing.Operation == commit.Request.Operation && existing.CaseID == commit.Request.CaseID {
				return nil
			}
			return domain.NewError(domain.CodeAlreadyExists, "请求标识已被其他写操作使用", "request_id")
		}
	}
	preparedEvents := make([]domain.AuditEvent, len(commit.Events))
	for i, event := range commit.Events {
		if event.CaseID != commit.Case.ID {
			return domain.Invalid("event.case_id", "审计事件案件标识不一致")
		}
		event.Sequence = s.nextSeq + int64(i)
		if event.DataVersion == 0 {
			event.DataVersion = domain.SnapshotVersion
		}
		if event.DataVersion != domain.SnapshotVersion {
			return domain.DataInvalid("event.data_version", "不支持的审计事件数据版本")
		}
		if event.CaseRevision == 0 {
			event.CaseRevision = commit.Case.Revision - int64(len(commit.Events)-1-i)
		}
		preparedEvents[i] = event
	}
	tempPath, err := prepareSnapshot(path, commit.Case)
	if err != nil {
		return err
	}
	// Keep the previous case snapshot so the commit chain can restore it when a
	// later write fails; for create there is nothing to restore.
	var caseBackup string
	if !creating {
		caseBackup, err = prepareSnapshot(path, current)
		if err != nil {
			return fmt.Errorf("备份案件快照: %w", err)
		}
		defer func() { _ = os.Remove(caseBackup) }()
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tempPath)
		}
	}()
	auditSize, err := s.appendEvents(preparedEvents)
	if err != nil {
		return err
	}
	if err := replaceSnapshot(tempPath, path); err != nil {
		_ = os.Truncate(s.auditPath, auditSize)
		return err
	}
	if creating {
		if err := replaceSnapshot(digestTemp, s.digestPath); err != nil {
			_ = os.Remove(path)
			_ = os.Truncate(s.auditPath, auditSize)
			return fmt.Errorf("保存内容摘要索引: %w", err)
		}
	}
	// The idempotency index is the final member of the atomic commit chain. Its
	// replacement must not leave the already-committed case snapshot, audit log
	// and digest index half-advanced on failure: restore every prior artefact to
	// its pre-call state before returning the error.
	var proposedRecord *RequestRecord
	var requestTemp string
	if commit.Request != nil {
		proposedRecord = &RequestRecord{RequestID: commit.Request.RequestID, Operation: commit.Request.Operation,
			CaseID: commit.Request.CaseID, Revision: commit.Case.Revision}
		proposedRecord.Result, err = commit.Case.Clone()
		if err != nil {
			s.restoreCommittedState(path, caseBackup, creating, auditSize)
			return fmt.Errorf("复制幂等结果: %w", err)
		}
		requestTemp, err = s.prepareRequestSnapshot(proposedRecord)
		if err != nil {
			s.restoreCommittedState(path, caseBackup, creating, auditSize)
			return fmt.Errorf("准备幂等索引: %w", err)
		}
		defer func() { _ = os.Remove(requestTemp) }()
		if err := replaceSnapshot(requestTemp, s.requestPath); err != nil {
			s.restoreCommittedState(path, caseBackup, creating, auditSize)
			return fmt.Errorf("保存幂等索引: %w", err)
		}
	}
	committed = true
	s.nextSeq += int64(len(preparedEvents))
	if creating {
		s.digests = nextDigests
	}
	if proposedRecord != nil {
		s.requests[proposedRecord.RequestID] = *proposedRecord
	}
	return nil
}

// restoreCommittedState undoes the durable writes performed before a later
// commit step failed. It is best-effort: every artefact is restored to its
// pre-call state so a failed request cannot expose a partially committed case
// (incremented revision, migrated status, appended audit records). The in-memory
// caches (s.digests, s.requests, s.nextSeq) are never updated before the whole
// commit chain succeeds, so they already reflect the pre-call state here.
func (s *DiskStore) restoreCommittedState(path, caseBackup string, creating bool, auditSize int64) {
	if creating {
		_ = os.Remove(path)
		if restorable, err := s.prepareDigestIndex(s.digests); err == nil {
			_ = replaceSnapshot(restorable, s.digestPath)
		}
	} else if caseBackup != "" {
		_ = replaceSnapshot(caseBackup, path)
	}
	_ = os.Truncate(s.auditPath, auditSize)
}

func (s *DiskStore) readCaseUnlocked(path string) (*domain.DisclosureCase, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c domain.DisclosureCase
	if err := jsonUnmarshalStrict(b, &c); err != nil {
		return nil, err
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func validateRequestRecord(r RequestRecord, c *domain.DisclosureCase) error {
	if r.RequestID == "" {
		return domain.Invalid("request_id", "请求标识不能为空")
	}
	if len(r.RequestID) > 128 {
		return domain.Invalid("request_id", "请求标识过长")
	}
	if r.Operation == "" {
		return domain.Invalid("operation", "操作名不能为空")
	}
	if r.CaseID != c.ID {
		return domain.Invalid("request.case_id", "幂等记录案件标识不一致")
	}
	return nil
}
