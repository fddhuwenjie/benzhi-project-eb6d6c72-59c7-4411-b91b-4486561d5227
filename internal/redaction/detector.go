package redaction

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"archive-review/internal/domain"
)

type Detector struct{ rules []Rule }

type candidate struct {
	start, end int
	rule       Rule
	text       string
}

func NewDetector(rules []Rule) *Detector {
	copyRules := append([]Rule(nil), rules...)
	return &Detector{rules: copyRules}
}

func NewDefaultDetector() *Detector { return NewDetector(DefaultRules()) }

func (d *Detector) Detect(caseID, content string) ([]domain.SensitiveFinding, error) {
	if caseID == "" {
		return nil, domain.Invalid("case_id", "案件标识不能为空")
	}
	candidates := make([]candidate, 0)
	for _, rule := range d.rules {
		matches := rule.Pattern.FindAllStringSubmatchIndex(content, -1)
		for _, indexes := range matches {
			start, end := indexes[0], indexes[1]
			if len(indexes) >= 4 && indexes[2] >= 0 {
				start, end = indexes[2], indexes[3]
			}
			if start >= 0 && end > start {
				candidates = append(candidates, candidate{start: start, end: end, rule: rule, text: content[start:end]})
			}
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].start != candidates[j].start {
			return candidates[i].start < candidates[j].start
		}
		if candidates[i].end != candidates[j].end {
			return candidates[i].end > candidates[j].end
		}
		return candidates[i].rule.ID < candidates[j].rule.ID
	})
	merged := mergeCandidates(candidates)
	findings := make([]domain.SensitiveFinding, 0, len(merged))
	for i, item := range merged {
		item.text = content[item.start:item.end]
		textDigest := domain.DigestString(item.text)
		idSource := fmt.Sprintf("%s:%d:%d:%s:%s", caseID, item.start, item.end, item.rule.ID, textDigest)
		sum := sha256.Sum256([]byte(idSource))
		finding := domain.SensitiveFinding{
			ID: "fnd_" + hex.EncodeToString(sum[:8]), CaseID: caseID, StartOffset: item.start, EndOffset: item.end,
			Category: item.rule.Category, MatchedTextDigest: textDigest, Confidence: item.rule.Confidence,
			RuleID: item.rule.ID, RuleBasis: item.rule.Basis, Decision: domain.DecisionPending,
		}
		if err := finding.Validate(len(content)); err != nil {
			return nil, fmt.Errorf("检测结果 %d 无效: %w", i, err)
		}
		findings = append(findings, finding)
	}
	return findings, nil
}
