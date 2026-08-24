package redaction

import (
	"encoding/json"
	"sort"

	"archive-review/internal/domain"
)

func BuildManifest(c *domain.DisclosureCase) (*domain.PublicationManifest, error) {
	if c.Status != domain.StatusApproved && c.Status != domain.StatusPublished {
		return nil, domain.InvalidState("仅已批准案件可生成发布清单")
	}
	approval := c.LatestApproval()
	if approval == nil {
		return nil, domain.Invalid("review", "缺少批准复核记录")
	}
	rendered, err := Render(c.ContentExcerpt, c.Findings)
	if err != nil {
		return nil, err
	}
	ordered := append([]domain.SensitiveFinding(nil), c.Findings...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].StartOffset != ordered[j].StartOffset {
			return ordered[i].StartOffset < ordered[j].StartOffset
		}
		return ordered[i].ID < ordered[j].ID
	})
	redactions := make([]domain.ManifestRedaction, 0, len(ordered))
	for _, f := range ordered {
		redactions = append(redactions, domain.ManifestRedaction{FindingID: f.ID, Category: f.Category,
			Start: f.StartOffset, End: f.EndOffset, Decision: f.Decision, Replacement: f.Replacement})
	}
	manifest := &domain.PublicationManifest{Version: 1, CaseID: c.ID, Title: c.Title,
		SourceDepartment: c.SourceDepartment, SourceContentDigest: c.ContentDigest, PublishedContent: rendered,
		ContentFingerprint: domain.DigestString(rendered), Redactions: redactions,
		ApprovedBy: approval.ReviewerID, ApprovalReason: approval.Reason}
	b, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	manifest.ManifestDigest = domain.DigestString(string(b))
	return manifest, nil
}
