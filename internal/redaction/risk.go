package redaction

import (
	"sort"
	"unicode/utf8"

	"archive-review/internal/domain"
)

func BuildRiskSummary(findings []domain.SensitiveFinding, detectionRevision, caseRevision int64) (*domain.RiskSummary, error) {
	return buildRiskSummary("", findings, detectionRevision, caseRevision)
}

func BuildRiskSummaryForContent(content string, findings []domain.SensitiveFinding, detectionRevision, caseRevision int64) (*domain.RiskSummary, error) {
	return buildRiskSummary(content, findings, detectionRevision, caseRevision)
}

func buildRiskSummary(content string, findings []domain.SensitiveFinding, detectionRevision, caseRevision int64) (*domain.RiskSummary, error) {
	ordered := append([]domain.SensitiveFinding(nil), findings...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].StartOffset != ordered[j].StartOffset {
			return ordered[i].StartOffset < ordered[j].StartOffset
		}
		if ordered[i].EndOffset != ordered[j].EndOffset {
			return ordered[i].EndOffset < ordered[j].EndOffset
		}
		return ordered[i].ID < ordered[j].ID
	})
	summary := &domain.RiskSummary{DetectionRevision: detectionRevision, CaseRevision: caseRevision,
		FindingsDigest: domain.FindingsEvidenceDigest(findings), TotalFindings: len(findings),
		ByCategory: map[domain.FindingCategory]int{}, ByConfidence: map[domain.Confidence]int{},
		ByRule: map[string]int{}, RiskLevel: domain.RiskLow, RiskBasis: []domain.RiskBasis{}}
	cursor := -1
	hasHighIdentity, hasRestricted, hasContact := false, false, false
	for _, finding := range ordered {
		if !finding.Category.Valid() {
			return nil, domain.DataInvalid("finding.category", "风险摘要包含无效敏感类别")
		}
		if !finding.Confidence.Valid() {
			return nil, domain.DataInvalid("finding.confidence", "风险摘要包含无效置信等级")
		}
		summary.ByCategory[finding.Category]++
		summary.ByConfidence[finding.Confidence]++
		summary.ByRule[finding.RuleID]++
		switch finding.Decision {
		case domain.DecisionPending:
			summary.Decisions.Pending++
		case domain.DecisionAccept:
			summary.Decisions.Accepted++
		case domain.DecisionAdjust:
			summary.Decisions.Adjusted++
		case domain.DecisionReject:
			summary.Decisions.Rejected++
		default:
			return nil, domain.DataInvalid("finding.decision", "风险摘要包含无效处置状态")
		}
		if finding.StartOffset >= cursor {
			summary.CoveredCharacters += intervalLength(content, finding.StartOffset, finding.EndOffset)
			cursor = finding.EndOffset
		} else if finding.EndOffset > cursor {
			summary.CoveredCharacters += intervalLength(content, cursor, finding.EndOffset)
			cursor = finding.EndOffset
		}
		hasHighIdentity = hasHighIdentity || (finding.Category == domain.CategoryIdentity && finding.Confidence == domain.ConfidenceHigh)
		hasRestricted = hasRestricted || finding.Category == domain.CategoryRestricted
		hasContact = hasContact || finding.Category == domain.CategoryContact
	}
	for _, finding := range ordered {
		code := ""
		if finding.Category == domain.CategoryIdentity && finding.Confidence == domain.ConfidenceHigh {
			code = "high_confidence_identity"
		} else if finding.Category == domain.CategoryRestricted {
			code = "restricted_identifier"
		} else if finding.Category == domain.CategoryContact {
			code = "contact_information"
		}
		if code != "" {
			summary.RiskBasis = append(summary.RiskBasis, domain.RiskBasis{Code: code, RuleID: finding.RuleID, Message: finding.RuleBasis})
		}
	}
	switch {
	case hasRestricted || (hasHighIdentity && hasContact):
		summary.RiskLevel = domain.RiskHigh
	case hasHighIdentity || hasContact || len(findings) > 0:
		summary.RiskLevel = domain.RiskMedium
		if len(summary.RiskBasis) == 0 {
			for _, finding := range ordered {
				summary.RiskBasis = append(summary.RiskBasis, domain.RiskBasis{Code: "sensitive_rule_match", RuleID: finding.RuleID, Message: finding.RuleBasis})
			}
		}
	default:
		summary.RiskLevel = domain.RiskLow
		summary.RiskBasis = append(summary.RiskBasis, domain.RiskBasis{Code: "no_sensitive_findings", Message: "未检测到敏感发现项"})
	}
	return summary, nil
}

func intervalLength(content string, start, end int) int {
	if content == "" || start < 0 || end > len(content) {
		return end - start
	}
	return utf8.RuneCountInString(content[start:end])
}

func ValidateLayout(content string, findings []domain.SensitiveFinding) error {
	ordered := append([]domain.SensitiveFinding(nil), findings...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].StartOffset != ordered[j].StartOffset {
			return ordered[i].StartOffset < ordered[j].StartOffset
		}
		return ordered[i].ID < ordered[j].ID
	})
	cursor := 0
	for _, finding := range ordered {
		if err := finding.Validate(len(content)); err != nil {
			return err
		}
		if finding.StartOffset < cursor {
			return domain.Invalid("findings", "发现项区间相互重叠")
		}
		cursor = finding.EndOffset
	}
	return nil
}
