package redaction

import "archive-review/internal/domain"

func mergeCandidates(items []candidate) []candidate {
	if len(items) == 0 {
		return []candidate{}
	}
	result := make([]candidate, 0, len(items))
	for _, current := range items {
		if len(result) == 0 {
			result = append(result, current)
			continue
		}
		last := &result[len(result)-1]
		if current.start >= last.end {
			result = append(result, current)
			continue
		}
		winner := stronger(*last, current)
		start, end := last.start, last.end
		if current.start < start {
			start = current.start
		}
		if current.end > end {
			end = current.end
		}
		winner.start, winner.end = start, end
		lastText := winner.text
		if end-start != len(lastText) {
			// The detector replaces this with the exact source slice after merging.
			lastText = ""
		}
		winner.text = lastText
		*last = winner
	}
	return result
}

func stronger(a, b candidate) candidate {
	weight := func(c domain.Confidence) int {
		switch c {
		case domain.ConfidenceHigh:
			return 3
		case domain.ConfidenceMedium:
			return 2
		default:
			return 1
		}
	}
	if weight(b.rule.Confidence) > weight(a.rule.Confidence) {
		return b
	}
	if weight(b.rule.Confidence) == weight(a.rule.Confidence) && b.rule.ID < a.rule.ID {
		return b
	}
	return a
}
