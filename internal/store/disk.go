package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"archive-review/internal/domain"
)

var safeID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,100}$`)

type DiskStore struct {
	root        string
	casesDir    string
	auditPath   string
	requestPath string
	digestPath  string
	mu          sync.Mutex
	requests    map[string]RequestRecord
	nextSeq     int64
	digests     map[string][]string
}

func OpenDisk(root string) (*DiskStore, error) {
	if root == "" {
		return nil, domain.Invalid("data_dir", "数据目录不能为空")
	}
	casesDir := filepath.Join(root, "cases")
	if err := os.MkdirAll(casesDir, 0o750); err != nil {
		return nil, fmt.Errorf("创建案件目录: %w", err)
	}
	s := &DiskStore{root: root, casesDir: casesDir, auditPath: filepath.Join(root, "audit.jsonl"),
		requestPath: filepath.Join(root, "requests.json"), digestPath: filepath.Join(root, "content-digests.json"),
		requests: map[string]RequestRecord{}, digests: map[string][]string{}, nextSeq: 1}
	if err := s.loadRequests(); err != nil {
		return nil, err
	}
	if err := s.loadDigestIndex(); err != nil {
		return nil, err
	}
	if err := s.loadNextSequence(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *DiskStore) Get(ctx context.Context, id string) (*domain.DisclosureCase, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !safeID.MatchString(id) {
		return nil, domain.Invalid("id", "案件标识格式无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.readCase(id)
	if err != nil {
		return nil, err
	}
	if err := s.validateDigestLink(c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *DiskStore) Create(ctx context.Context, commit Commit) error {
	return s.commit(ctx, commit, true)
}

func (s *DiskStore) Save(ctx context.Context, commit Commit) error {
	return s.commit(ctx, commit, false)
}

func (s *DiskStore) LookupRequest(ctx context.Context, requestID string) (*RequestRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.requests[requestID]
	if !ok {
		return nil, domain.NotFound("请求", requestID)
	}
	copy := record
	if record.Result != nil {
		var err error
		copy.Result, err = record.Result.Clone()
		if err != nil {
			return nil, err
		}
	}
	return &copy, nil
}

func (s *DiskStore) casePath(id string) string { return filepath.Join(s.casesDir, id+".json") }

func (s *DiskStore) readCase(id string) (*domain.DisclosureCase, error) {
	c, err := s.readCaseUnlocked(s.casePath(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, domain.NotFound("案件", id)
	}
	if err != nil {
		return nil, fmt.Errorf("读取案件快照: %w", err)
	}
	return c.Clone()
}

func (s *DiskStore) FindByContentDigest(ctx context.Context, digest string) ([]*domain.DisclosureCase, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := append([]string(nil), s.digests[digest]...)
	result := make([]*domain.DisclosureCase, 0, len(ids))
	for _, id := range ids {
		c, err := s.readCaseUnlocked(s.casePath(id))
		if err != nil {
			return nil, fmt.Errorf("内容摘要索引指向无效案件 %q: %w", id, err)
		}
		if c.ContentDigest != digest {
			return nil, domain.DataInvalid("content_digest_index", "内容摘要索引与案件快照不一致")
		}
		result = append(result, c)
	}
	return result, nil
}

func (s *DiskStore) AllCases(ctx context.Context) ([]*domain.DisclosureCase, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.casesDir)
	if err != nil {
		return nil, fmt.Errorf("枚举案件快照: %w", err)
	}
	result := make([]*domain.DisclosureCase, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if !safeID.MatchString(id) {
			return nil, domain.DataInvalid("case_id", "案件目录包含无效快照文件名")
		}
		c, err := s.readCaseUnlocked(s.casePath(id))
		if err != nil {
			return nil, fmt.Errorf("读取案件快照 %q: %w", id, err)
		}
		if err := s.validateDigestLink(c); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, nil
}

func (s *DiskStore) validateDigestLink(c *domain.DisclosureCase) error {
	found := false
	for _, id := range s.digests[c.ContentDigest] {
		if id == c.ID {
			found = true
		}
		indexed, err := s.readCaseUnlocked(s.casePath(id))
		if err != nil || indexed.ContentDigest != c.ContentDigest {
			return domain.DataInvalid("content_digest_index", "内容摘要索引与关联案件快照不一致")
		}
	}
	if found {
		return nil
	}
	return domain.DataInvalid("content_digest_index", "案件快照在内容摘要索引中缺失或失配")
}
