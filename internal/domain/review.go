package domain

import (
	"strings"
	"time"
)

type ReviewOutcome string

const (
	ReviewReturned ReviewOutcome = "returned"
	ReviewApproved ReviewOutcome = "approved"
)

type ReviewDecision struct {
	ID               string            `json:"id"`
	CaseID           string            `json:"case_id"`
	ReviewerID       string            `json:"reviewer_id"`
	Outcome          ReviewOutcome     `json:"outcome"`
	Reason           string            `json:"reason"`
	CaseRevision     int64             `json:"case_revision"`
	CreatedAt        time.Time         `json:"created_at"`
	RemediationItems []RemediationItem `json:"remediation_items,omitempty"`
}

type RemediationItem struct {
	FindingID   string `json:"finding_id"`
	Instruction string `json:"instruction"`
}

func (r ReviewDecision) Validate(submitterID string) error {
	if strings.TrimSpace(r.ID) == "" || strings.TrimSpace(r.CaseID) == "" || r.CaseRevision < 1 || r.CreatedAt.IsZero() {
		return Invalid("review", "复核记录关键证据不完整")
	}
	if strings.TrimSpace(r.ReviewerID) == "" {
		return Invalid("reviewer_id", "复核人不能为空")
	}
	if r.ReviewerID == submitterID {
		return Forbidden("提交人与复核人不得相同")
	}
	if strings.TrimSpace(r.Reason) == "" {
		return Invalid("reason", "复核理由不能为空")
	}
	if r.Outcome != ReviewReturned && r.Outcome != ReviewApproved {
		return Invalid("outcome", "复核结论无效")
	}
	if r.Outcome == ReviewApproved && len(r.RemediationItems) != 0 {
		return Invalid("remediation_items", "批准结论不得包含整改项")
	}
	seen := make(map[string]struct{}, len(r.RemediationItems))
	if len(r.RemediationItems) > 100 {
		return Invalid("remediation_items", "整改项数量不得超过 100")
	}
	for i, item := range r.RemediationItems {
		if strings.TrimSpace(item.FindingID) == "" {
			return Invalid("remediation_items", "整改项发现标识不能为空")
		}
		if strings.TrimSpace(item.Instruction) == "" {
			return Invalid("remediation_items", "整改意见不能为空")
		}
		if len([]rune(item.Instruction)) > 1000 {
			return Invalid("remediation_items", "单项整改意见不得超过 1000 个字符")
		}
		if _, ok := seen[item.FindingID]; ok {
			return Invalid("remediation_items", "整改项发现标识不得重复")
		}
		seen[item.FindingID] = struct{}{}
		_ = i
	}
	return nil
}
