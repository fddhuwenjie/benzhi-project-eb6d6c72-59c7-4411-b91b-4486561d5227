package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

const SnapshotVersion = 1

type DisclosureCase struct {
	DataVersion         int                          `json:"data_version"`
	ID                  string                       `json:"id"`
	Title               string                       `json:"title"`
	SourceDepartment    string                       `json:"source_department"`
	ContentDigest       string                       `json:"content_digest"`
	ContentExcerpt      string                       `json:"content_excerpt"`
	Status              CaseStatus                   `json:"status"`
	SubmitterID         string                       `json:"submitter_id"`
	ReviewerID          string                       `json:"reviewer_id,omitempty"`
	Revision            int64                        `json:"revision"`
	Findings            []SensitiveFinding           `json:"findings"`
	Reviews             []ReviewDecision             `json:"reviews"`
	Manifest            *PublicationManifest         `json:"manifest,omitempty"`
	DuplicateAcceptance *DuplicateAcceptanceEvidence `json:"duplicate_acceptance,omitempty"`
	RiskSummary         *RiskSummary                 `json:"risk_summary,omitempty"`
	CreatedAt           time.Time                    `json:"created_at"`
	UpdatedAt           time.Time                    `json:"updated_at"`
	PublishedAt         *time.Time                   `json:"published_at,omitempty"`
}

func NewCase(id, title, department, excerpt, submitter string, now time.Time) (*DisclosureCase, error) {
	id = strings.TrimSpace(id)
	title = strings.TrimSpace(title)
	department = strings.TrimSpace(department)
	excerpt = strings.TrimSpace(excerpt)
	submitter = strings.TrimSpace(submitter)
	if id == "" {
		return nil, Invalid("id", "案件标识不能为空")
	}
	if len([]rune(title)) < 2 || len([]rune(title)) > 200 {
		return nil, Invalid("title", "标题长度必须为 2 到 200 个字符")
	}
	if len([]rune(department)) < 2 || len([]rune(department)) > 120 {
		return nil, Invalid("source_department", "来源部门长度必须为 2 到 120 个字符")
	}
	if len([]rune(excerpt)) < 10 || len([]rune(excerpt)) > 100000 {
		return nil, Invalid("content_excerpt", "材料摘要长度必须为 10 到 100000 个字符")
	}
	if submitter == "" {
		return nil, Invalid("actor_id", "提交人不能为空")
	}
	t := now.UTC()
	return &DisclosureCase{DataVersion: SnapshotVersion, ID: id, Title: title, SourceDepartment: department,
		ContentDigest: DigestString(excerpt), ContentExcerpt: excerpt, Status: StatusDraft, SubmitterID: submitter,
		Revision: 1, Findings: []SensitiveFinding{}, Reviews: []ReviewDecision{}, CreatedAt: t, UpdatedAt: t}, nil
}

func (c *DisclosureCase) Validate() error {
	if c.DataVersion != SnapshotVersion {
		return DataInvalid("data_version", "不支持的快照数据版本")
	}
	if c.ID == "" || c.SubmitterID == "" {
		return Invalid("case", "案件关键标识缺失")
	}
	if !c.Status.Valid() {
		return Invalid("status", "案件状态无效")
	}
	if c.Revision < 1 {
		return Invalid("revision", "案件修订号无效")
	}
	if c.ContentDigest != DigestString(c.ContentExcerpt) {
		return DataInvalid("content_digest", "材料内容摘要校验失败")
	}
	for _, finding := range c.Findings {
		if err := finding.Validate(len(c.ContentExcerpt)); err != nil {
			return DataInvalid("findings", "案件发现项校验失败: "+err.Error())
		}
		if finding.CaseID != c.ID {
			return Invalid("finding.case_id", "发现项不属于当前案件")
		}
		if finding.EndOffset > len(c.ContentExcerpt) || DigestString(c.ContentExcerpt[finding.StartOffset:finding.EndOffset]) != finding.MatchedTextDigest {
			return DataInvalid("finding.matched_text_digest", "发现项与源内容摘要不一致")
		}
	}
	if c.Status != StatusDraft && c.RiskSummary == nil {
		return DataInvalid("risk_summary", "已完成检测的案件缺少敏感检测风险摘要")
	}
	if c.RiskSummary != nil {
		if c.RiskSummary.CaseRevision != c.Revision || c.RiskSummary.DetectionRevision < 1 || c.RiskSummary.DetectionRevision > c.Revision {
			return DataInvalid("risk_summary.case_revision", "风险摘要 revision 与案件快照不一致")
		}
		if c.RiskSummary.FindingsDigest != FindingsEvidenceDigest(c.Findings) || c.RiskSummary.TotalFindings != len(c.Findings) {
			return DataInvalid("risk_summary.findings_digest", "风险摘要所依据的检测结果与案件快照不一致")
		}
	}
	if c.DuplicateAcceptance != nil {
		if len(c.DuplicateAcceptance.RelatedCases) == 0 || c.DuplicateAcceptance.ReasonDigest == "" {
			return Invalid("duplicate_acceptance", "重复受理证据不完整")
		}
		for _, related := range c.DuplicateAcceptance.RelatedCases {
			if related.CaseID == "" || !related.Status.Valid() || related.SourceDepartment == "" ||
				(related.DuplicateKind != "same_department" && related.DuplicateKind != "cross_department") {
				return Invalid("duplicate_acceptance.related_cases", "重复受理关联案件证据无效")
			}
		}
	}
	for _, review := range c.Reviews {
		if review.CaseID != c.ID {
			return Invalid("review.case_id", "复核记录不属于当前案件")
		}
		if err := review.Validate(c.SubmitterID); err != nil {
			return err
		}
	}
	if (c.Status == StatusApproved || c.Status == StatusPublished) && len(c.Reviews) == 0 {
		return Invalid("reviews", "已批准案件缺少复核记录")
	}
	if c.Status == StatusPublished && (c.Manifest == nil || c.PublishedAt == nil) {
		return Invalid("manifest", "已发布案件缺少发布证据")
	}
	return nil
}

func (c *DisclosureCase) Clone() (*DisclosureCase, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	var clone DisclosureCase
	if err := json.Unmarshal(b, &clone); err != nil {
		return nil, err
	}
	return &clone, nil
}

func (c *DisclosureCase) Transition(to CaseStatus, now time.Time) error {
	if !CanTransition(c.Status, to) {
		return InvalidState("不允许从 " + string(c.Status) + " 变更为 " + string(to))
	}
	c.Status = to
	c.Revision++
	c.UpdatedAt = now.UTC()
	return nil
}

func (c *DisclosureCase) Touch(now time.Time) {
	c.Revision++
	c.UpdatedAt = now.UTC()
}

func (c *DisclosureCase) AllFindingsDecided() bool {
	for _, finding := range c.Findings {
		if !finding.IsDecided() {
			return false
		}
	}
	return true
}

func (c *DisclosureCase) LatestApproval() *ReviewDecision {
	for i := len(c.Reviews) - 1; i >= 0; i-- {
		if c.Reviews[i].Outcome == ReviewApproved {
			return &c.Reviews[i]
		}
	}
	return nil
}

func (c *DisclosureCase) Mutable() error {
	if c.Status == StatusApproved || c.Status == StatusPublished {
		return InvalidState("案件已冻结，不允许修改")
	}
	return nil
}

func DigestString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func PayloadDigest(value any) string {
	b, err := json.Marshal(value)
	if err != nil {
		return DigestString("marshal-error")
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
