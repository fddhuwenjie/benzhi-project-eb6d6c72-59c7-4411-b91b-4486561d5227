package detector_cache_alias_test

import (
	"testing"

	"archive-review/internal/redaction"
)

func TestDetectorCacheKeepsResultsIsolatedByCase(t *testing.T) {
	detector := redaction.NewDefaultDetector()
	content := "姓名：张三，联系电话 13800138000，该材料用于缓存隔离测试。"

	first, err := detector.Detect("case-first", content)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) == 0 {
		t.Fatal("测试材料应产生敏感发现项")
	}
	firstID := first[0].ID

	second, err := detector.Detect("case-second", content)
	if err != nil {
		t.Fatal(err)
	}
	if second[0].CaseID != "case-second" || second[0].ID == firstID {
		t.Fatalf("第二个案件未获得独立身份: %+v", second[0])
	}
	if first[0].CaseID != "case-first" || first[0].ID != firstID {
		t.Fatalf("第一次检测结果被后续缓存复用污染: %+v", first[0])
	}
}
