package domain

import (
	"sort"
	"strings"
)

type DuplicateCaseSummary struct {
	CaseID           string     `json:"case_id"`
	Status           CaseStatus `json:"status"`
	SourceDepartment string     `json:"source_department"`
	DuplicateKind    string     `json:"duplicate_kind"`
}

type DuplicateAcceptanceEvidence struct {
	RelatedCases []DuplicateCaseSummary `json:"related_cases"`
	ReasonDigest string                 `json:"reason_digest"`
}

func ValidateDuplicateAcceptance(allow bool, reason string, related []DuplicateCaseSummary) error {
	if len(related) == 0 {
		return nil
	}
	if !allow {
		return NewDetailedError(CodeDuplicate, "存在未归档的相同材料，继续受理前必须明确允许重复", "allow_duplicate", related)
	}
	if strings.TrimSpace(reason) == "" {
		return Invalid("duplicate_reason", "继续受理重复材料时必须填写理由")
	}
	return nil
}

type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

type DecisionCounts struct {
	Pending  int `json:"pending"`
	Accepted int `json:"accepted"`
	Adjusted int `json:"adjusted"`
	Rejected int `json:"rejected"`
}

type RiskBasis struct {
	Code    string `json:"code"`
	RuleID  string `json:"rule_id,omitempty"`
	Message string `json:"message"`
}

type RiskSummary struct {
	DetectionRevision int64                   `json:"detection_revision"`
	CaseRevision      int64                   `json:"case_revision"`
	FindingsDigest    string                  `json:"findings_digest"`
	TotalFindings     int                     `json:"total_findings"`
	CoveredCharacters int                     `json:"covered_characters"`
	ByCategory        map[FindingCategory]int `json:"by_category"`
	ByConfidence      map[Confidence]int      `json:"by_confidence"`
	ByRule            map[string]int          `json:"by_rule"`
	Decisions         DecisionCounts          `json:"decisions"`
	RiskLevel         RiskLevel               `json:"risk_level"`
	RiskBasis         []RiskBasis             `json:"risk_basis"`
}

type findingEvidence struct {
	ID                string          `json:"id"`
	Start             int             `json:"start"`
	End               int             `json:"end"`
	Category          FindingCategory `json:"category"`
	Confidence        Confidence      `json:"confidence"`
	RuleID            string          `json:"rule_id"`
	MatchedTextDigest string          `json:"matched_text_digest"`
}

func FindingsEvidenceDigest(findings []SensitiveFinding) string {
	items := make([]findingEvidence, 0, len(findings))
	for _, f := range findings {
		items = append(items, findingEvidence{f.ID, f.StartOffset, f.EndOffset, f.Category, f.Confidence, f.RuleID, f.MatchedTextDigest})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return PayloadDigest(items)
}

type ReadinessBlocker struct {
	Field     string `json:"field"`
	FindingID string `json:"finding_id,omitempty"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

type ReviewEvidenceSummary struct {
	Decisions          DecisionCounts `json:"decisions"`
	ContentFingerprint string         `json:"content_fingerprint"`
}

type ReviewReadiness struct {
	Ready             bool                   `json:"ready"`
	PreflightRevision int64                  `json:"preflight_revision"`
	Blockers          []ReadinessBlocker     `json:"blockers"`
	Evidence          *ReviewEvidenceSummary `json:"evidence,omitempty"`
}

type ManifestPreview struct {
	PublishedContent   string `json:"published_content"`
	RedactionCount     int    `json:"redaction_count"`
	ContentFingerprint string `json:"content_fingerprint"`
	ManifestDigest     string `json:"manifest_digest"`
	PreviewRevision    int64  `json:"preview_revision"`
}

type IntegrityIssue struct {
	Code     string `json:"code"`
	Field    string `json:"field,omitempty"`
	Sequence int64  `json:"sequence,omitempty"`
	Message  string `json:"message"`
}

type IntegrityResult struct {
	Passed             bool             `json:"passed"`
	Verifiable         bool             `json:"verifiable"`
	FirstSequence      int64            `json:"first_sequence,omitempty"`
	LastSequence       int64            `json:"last_sequence,omitempty"`
	SnapshotRevision   int64            `json:"snapshot_revision"`
	ContentFingerprint string           `json:"content_fingerprint,omitempty"`
	ManifestDigest     string           `json:"manifest_digest,omitempty"`
	Issues             []IntegrityIssue `json:"issues"`
}
