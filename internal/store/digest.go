package store

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

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
	return s.validateDigestIndexConsistency()
}

// validateDigestIndexConsistency cross-checks the in-memory digest index against the
// case snapshots on disk so that corrupt persistent state is rejected at startup rather
// than deferred until a case is read. It ensures every indexed case identifier resolves
// to a readable snapshot whose ContentDigest matches the index entry, and that every
// case snapshot is present in the index with a matching digest.
func (s *DiskStore) validateDigestIndexConsistency() error {
	visited := make(map[string]struct{}, len(s.digests))
	entries, err := os.ReadDir(s.casesDir)
	if err != nil {
		return fmt.Errorf("枚举案件快照: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if !safeID.MatchString(id) {
			return domain.DataInvalid("case_id", "案件目录包含无效快照文件名")
		}
		c, err := s.readCaseUnlocked(s.casePath(id))
		if err != nil {
			return domain.DataInvalid("case_snapshot", fmt.Sprintf("读取案件快照 %q 失败: %v", id, err))
		}
		indexed, ok := s.digests[c.ContentDigest]
		if !ok {
			return domain.DataInvalid("content_digest_index", fmt.Sprintf("案件快照 %q 的内容摘要未在索引中登记", id))
		}
		found := false
		for _, indexedID := range indexed {
			if indexedID == id {
				found = true
				break
			}
		}
		if !found {
			return domain.DataInvalid("content_digest_index", fmt.Sprintf("案件快照 %q 未在内容摘要索引中登记", id))
		}
		visited[id] = struct{}{}
	}
	for digest, ids := range s.digests {
		for _, id := range ids {
			if _, ok := visited[id]; !ok {
				return domain.DataInvalid("content_digest_index", fmt.Sprintf("内容摘要索引条目（摘要 %q）指向不存在的案件快照 %q", digest, id))
			}
			c, err := s.readCaseUnlocked(s.casePath(id))
			if err != nil {
				return domain.DataInvalid("content_digest_index", fmt.Sprintf("内容摘要索引指向无法读取的案件快照 %q: %v", id, err))
			}
			if c.ContentDigest != digest {
				return domain.DataInvalid("content_digest_index", fmt.Sprintf("案件快照 %q 的内容摘要与索引条目不一致", id))
			}
		}
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
