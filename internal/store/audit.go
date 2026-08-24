package store

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"archive-review/internal/domain"
)

func (s *DiskStore) appendEvents(events []domain.AuditEvent) (int64, error) {
	if len(events) == 0 {
		info, err := os.Stat(s.auditPath)
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		if err != nil {
			return 0, err
		}
		return info.Size(), nil
	}
	f, err := os.OpenFile(s.auditPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		return 0, fmt.Errorf("打开审计日志: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	originalSize := info.Size()
	for _, event := range events {
		b, err := json.Marshal(event)
		if err != nil {
			_ = os.Truncate(s.auditPath, originalSize)
			return originalSize, err
		}
		if _, err := f.Write(append(b, '\n')); err != nil {
			_ = os.Truncate(s.auditPath, originalSize)
			return originalSize, err
		}
	}
	if err := f.Sync(); err != nil {
		_ = os.Truncate(s.auditPath, originalSize)
		return originalSize, err
	}
	return originalSize, nil
}

func (s *DiskStore) Events(ctx context.Context, caseID string) ([]domain.AuditEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !safeID.MatchString(caseID) {
		return nil, domain.Invalid("case_id", "案件标识格式无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	all, err := readAuditFile(s.auditPath)
	if err != nil {
		return nil, err
	}
	filtered := make([]domain.AuditEvent, 0)
	for _, event := range all {
		if event.CaseID == caseID {
			filtered = append(filtered, event)
		}
	}
	if len(filtered) == 0 {
		if _, err := s.readCaseUnlocked(s.casePath(caseID)); errors.Is(err, os.ErrNotExist) {
			return nil, domain.NotFound("案件", caseID)
		} else if err != nil {
			return nil, fmt.Errorf("读取案件快照: %w", err)
		}
	}
	return filtered, nil
}

func (s *DiskStore) loadNextSequence() error {
	events, err := readAuditFile(s.auditPath)
	if err != nil {
		return err
	}
	var max int64
	for _, event := range events {
		if event.Sequence != max+1 {
			return fmt.Errorf("审计序列不连续或乱序: %d", event.Sequence)
		}
		max = event.Sequence
	}
	s.nextSeq = max + 1
	return nil
}

func readAuditFile(path string) ([]domain.AuditEvent, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return []domain.AuditEvent{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("打开审计日志: %w", err)
	}
	defer f.Close()
	reader := bufio.NewReader(f)
	events := make([]domain.AuditEvent, 0)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			var event domain.AuditEvent
			if decodeErr := jsonUnmarshalStrict(line, &event); decodeErr != nil {
				return nil, fmt.Errorf("解析审计日志: %w", decodeErr)
			}
			if event.DataVersion != domain.SnapshotVersion {
				return nil, domain.DataInvalid("audit_event.data_version", "审计日志包含不支持的数据版本")
			}
			events = append(events, event)
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	return events, nil
}
