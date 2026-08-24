package redaction

import (
	"sort"
	"strings"

	"archive-review/internal/domain"
)

func Render(content string, findings []domain.SensitiveFinding) (string, error) {
	if err := ValidateLayout(content, findings); err != nil {
		return "", err
	}
	ordered := append([]domain.SensitiveFinding(nil), findings...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].StartOffset < ordered[j].StartOffset })
	var out strings.Builder
	cursor := 0
	for _, finding := range ordered {
		if !finding.IsDecided() {
			return "", domain.Invalid("findings", "存在未决发现项")
		}
		out.WriteString(content[cursor:finding.StartOffset])
		switch finding.Decision {
		case domain.DecisionAccept, domain.DecisionAdjust:
			out.WriteString(finding.Replacement)
		case domain.DecisionReject:
			out.WriteString(content[finding.StartOffset:finding.EndOffset])
		default:
			return "", domain.Invalid("decision", "发现项决定无效")
		}
		cursor = finding.EndOffset
	}
	out.WriteString(content[cursor:])
	return out.String(), nil
}
