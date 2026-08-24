package domain

import (
	"strings"
	"time"
)

type FindingCategory string

const (
	CategoryIdentity   FindingCategory = "personal_identity"
	CategoryContact    FindingCategory = "contact_information"
	CategoryRestricted FindingCategory = "restricted_identifier"
)

type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

type FindingDecision string

const (
	DecisionPending FindingDecision = "pending"
	DecisionAccept  FindingDecision = "accept"
	DecisionAdjust  FindingDecision = "adjust"
	DecisionReject  FindingDecision = "reject"
)

type SensitiveFinding struct {
	ID                string          `json:"id"`
	CaseID            string          `json:"case_id"`
	StartOffset       int             `json:"start_offset"`
	EndOffset         int             `json:"end_offset"`
	Category          FindingCategory `json:"category"`
	MatchedTextDigest string          `json:"matched_text_digest"`
	Confidence        Confidence      `json:"confidence"`
	RuleID            string          `json:"rule_id"`
	RuleBasis         string          `json:"rule_basis"`
	Decision          FindingDecision `json:"decision"`
	Replacement       string          `json:"replacement,omitempty"`
	DecisionReason    string          `json:"decision_reason,omitempty"`
	DecidedBy         string          `json:"decided_by,omitempty"`
	DecidedAt         *time.Time      `json:"decided_at,omitempty"`
	DecisionRevision  int64           `json:"decision_revision,omitempty"`
}

func (c FindingCategory) Valid() bool {
	return c == CategoryIdentity || c == CategoryContact || c == CategoryRestricted
}

func (c Confidence) Valid() bool {
	return c == ConfidenceHigh || c == ConfidenceMedium || c == ConfidenceLow
}

func (d FindingDecision) Valid() bool {
	return d == DecisionPending || d == DecisionAccept || d == DecisionAdjust || d == DecisionReject
}

func (f SensitiveFinding) Validate(contentLength int) error {
	if strings.TrimSpace(f.ID) == "" {
		return Invalid("finding.id", "发现项标识不能为空")
	}
	if f.StartOffset < 0 || f.EndOffset <= f.StartOffset || f.EndOffset > contentLength {
		return Invalid("finding.offset", "发现项字符区间无效")
	}
	if f.RuleID == "" || f.MatchedTextDigest == "" {
		return Invalid("finding.rule_id", "发现项缺少规则证据")
	}
	if !f.Category.Valid() {
		return Invalid("finding.category", "发现项敏感类别无效")
	}
	if !f.Confidence.Valid() {
		return Invalid("finding.confidence", "发现项置信等级无效")
	}
	if !f.Decision.Valid() {
		return Invalid("finding.decision", "发现项决定无效")
	}
	if f.Decision == DecisionPending {
		if f.Replacement != "" || f.DecisionReason != "" || f.DecidedBy != "" || f.DecidedAt != nil || f.DecisionRevision != 0 {
			return Invalid("finding.decision", "待决发现项不得包含处置证据")
		}
	} else {
		if strings.TrimSpace(f.DecisionReason) == "" || strings.TrimSpace(f.DecidedBy) == "" || f.DecidedAt == nil {
			return Invalid("finding.decision", "已处理发现项缺少理由或操作者证据")
		}
		switch f.Decision {
		case DecisionAccept, DecisionAdjust:
			if strings.TrimSpace(f.Replacement) == "" {
				return Invalid("finding.replacement", "遮蔽决定缺少替换内容")
			}
		case DecisionReject:
			if f.Replacement != "" {
				return Invalid("finding.replacement", "驳回发现项不得包含替换内容")
			}
		}
		if len([]rune(f.Replacement)) > 500 || strings.ContainsAny(f.Replacement, "\r\n") {
			return Invalid("finding.replacement", "替换内容格式无效")
		}
	}
	return nil
}

func (f *SensitiveFinding) Reset() {
	f.Decision = DecisionPending
	f.Replacement = ""
	f.DecisionReason = ""
	f.DecidedBy = ""
	f.DecidedAt = nil
	f.DecisionRevision = 0
}

func (f SensitiveFinding) IsDecided() bool { return f.Decision != DecisionPending }

func (f *SensitiveFinding) Decide(decision FindingDecision, replacement, reason, actor string, now time.Time) error {
	if f.Decision != DecisionPending {
		return InvalidState("发现项已经完成决策")
	}
	if strings.TrimSpace(actor) == "" {
		return Invalid("actor_id", "操作者不能为空")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Invalid("reason", "处理理由不能为空")
	}
	switch decision {
	case DecisionAccept:
		if replacement == "" {
			replacement = "[已遮蔽]"
		}
	case DecisionAdjust:
		if strings.TrimSpace(replacement) == "" {
			return Invalid("replacement", "调整遮蔽时必须提供替换内容")
		}
	case DecisionReject:
		if replacement != "" {
			return Invalid("replacement", "驳回发现项时不得提供替换内容")
		}
	default:
		return Invalid("decision", "不支持的处理决定")
	}
	if len([]rune(replacement)) > 500 || strings.ContainsAny(replacement, "\r\n") {
		return Invalid("replacement", "替换内容不得超过 500 个字符或包含换行")
	}
	t := now.UTC()
	f.Decision = decision
	f.Replacement = replacement
	f.DecisionReason = reason
	f.DecidedBy = actor
	f.DecidedAt = &t
	return nil
}
