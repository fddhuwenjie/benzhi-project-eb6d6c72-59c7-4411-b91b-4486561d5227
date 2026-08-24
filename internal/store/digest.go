package store

import (
	"errors"
	"fmt"
	"os"
	"sort"

	"archive-review/internal/domain"
)

type digestEntry struct {
	Digest  string   `json:"digest"`
	CaseIDs []string `json:"case_ids"`
}

type digestIndex struct {
	DataVersion int           `json:"data_version"`
	Entries     []digestEntry `json:"entries"`
}

func (s *DiskStore) loadDigestIndex() error {
	b, err := os.ReadFile(s.digestPath)
	if errors.Is(err, os.ErrNotExist) {
		entries, readErr := os.ReadDir(s.casesDir)
		if readErr != nil {
			return readErr
		}
		for _, entry := range entries {
			if !entry.IsDir() && len(entry.Name()) > 5 && entry.Name()[len(entry.Name())-5:] == ".json" {
				return domain.DataInvalid("content_digest_index", "已有案件快照但内容摘要索引缺失")
			}
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取内容摘要索引: %w", err)
	}
	var index digestIndex
	if err := jsonUnmarshalStrict(b, &index); err != nil {
		return fmt.Errorf("解析内容摘要索引: %w", err)
	}
	if index.DataVersion != domain.SnapshotVersion {
		return domain.DataInvalid("content_digest_index.data_version", "不支持的内容摘要索引版本")
	}
	seenCases := map[string]struct{}{}
	for _, entry := range index.Entries {
		if entry.Digest == "" || len(entry.CaseIDs) == 0 {
			return domain.DataInvalid("content_digest_index", "内容摘要索引包含空条目")
		}
		if _, exists := s.digests[entry.Digest]; exists {
			return domain.DataInvalid("content_digest_index", "内容摘要索引包含重复摘要")
		}
		ids := append([]string(nil), entry.CaseIDs...)
		sort.Strings(ids)
		for _, id := range ids {
			if !safeID.MatchString(id) {
				return domain.DataInvalid("content_digest_index", "内容摘要索引包含无效案件标识")
			}
			if _, exists := seenCases[id]; exists {
				return domain.DataInvalid("content_digest_index", "案件在内容摘要索引中重复出现")
			}
			seenCases[id] = struct{}{}
		}
		s.digests[entry.Digest] = ids
	}
	return nil
}

func (s *DiskStore) prepareDigestIndex(digests map[string][]string) (string, error) {
	keys := make([]string, 0, len(digests))
	for digest := range digests {
		keys = append(keys, digest)
	}
	sort.Strings(keys)
	entries := make([]digestEntry, 0, len(keys))
	for _, digest := range keys {
		ids := append([]string(nil), digests[digest]...)
		sort.Strings(ids)
		entries = append(entries, digestEntry{Digest: digest, CaseIDs: ids})
	}
	return prepareSnapshot(s.digestPath, digestIndex{DataVersion: domain.SnapshotVersion, Entries: entries})
}

func cloneDigests(source map[string][]string) map[string][]string {
	result := make(map[string][]string, len(source))
	for digest, ids := range source {
		result[digest] = append([]string(nil), ids...)
	}
	return result
}
