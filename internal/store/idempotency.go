package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"archive-review/internal/domain"
)

type requestIndex struct {
	DataVersion int             `json:"data_version"`
	Records     []RequestRecord `json:"records"`
}

func (s *DiskStore) loadRequests() error {
	b, err := os.ReadFile(s.requestPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取幂等索引: %w", err)
	}
	var index requestIndex
	if err := json.Unmarshal(b, &index); err != nil {
		return fmt.Errorf("解析幂等索引: %w", err)
	}
	if index.DataVersion != domain.SnapshotVersion {
		return fmt.Errorf("不支持的幂等索引版本 %d", index.DataVersion)
	}
	for _, record := range index.Records {
		if record.RequestID == "" || record.CaseID == "" || record.Operation == "" || record.Result == nil {
			return fmt.Errorf("幂等索引包含无效记录")
		}
		if err := record.Result.Validate(); err != nil {
			return fmt.Errorf("幂等结果校验失败: %w", err)
		}
		if _, exists := s.requests[record.RequestID]; exists {
			return fmt.Errorf("幂等索引包含重复请求标识 %q", record.RequestID)
		}
		s.requests[record.RequestID] = record
	}
	return nil
}

func (s *DiskStore) persistRequests() error {
	records := make([]RequestRecord, 0, len(s.requests))
	for _, record := range s.requests {
		records = append(records, record)
	}
	// Stable order keeps the index diffable and deterministic.
	sortRequestRecords(records)
	temp, err := prepareSnapshot(s.requestPath, requestIndex{DataVersion: domain.SnapshotVersion, Records: records})
	if err != nil {
		return err
	}
	return replaceSnapshot(temp, s.requestPath)
}

func sortRequestRecords(records []RequestRecord) {
	for i := 1; i < len(records); i++ {
		for j := i; j > 0 && records[j].RequestID < records[j-1].RequestID; j-- {
			records[j], records[j-1] = records[j-1], records[j]
		}
	}
}
