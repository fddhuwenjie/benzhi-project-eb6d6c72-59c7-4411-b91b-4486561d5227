package workflow

import (
	"context"
	"sort"
	"strings"

	"archive-review/internal/domain"
)

const opCreate = "case.create"

type CreateInput struct {
	Title            string `json:"title"`
	SourceDepartment string `json:"source_department"`
	ContentExcerpt   string `json:"content_excerpt"`
	ContentDigest    string `json:"content_digest,omitempty"`
	AllowDuplicate   bool   `json:"allow_duplicate,omitempty"`
	DuplicateReason  string `json:"duplicate_reason,omitempty"`
}

func (s *Service) CreateCase(ctx context.Context, meta WriteMeta, input CreateInput) (*domain.DisclosureCase, error) {
	if err := meta.Validate(false); err != nil {
		return nil, err
	}
	input.Title = strings.TrimSpace(input.Title)
	input.SourceDepartment = strings.TrimSpace(input.SourceDepartment)
	input.ContentExcerpt = strings.TrimSpace(input.ContentExcerpt)
	input.ContentDigest = strings.TrimSpace(input.ContentDigest)
	input.DuplicateReason = strings.TrimSpace(input.DuplicateReason)
	operation := opCreate + ":" + domain.PayloadDigest(input)
	if replay, ok, err := s.replay(ctx, operation, "", meta.RequestID); ok || err != nil {
		return replay, err
	}
	if supplied := strings.TrimSpace(input.ContentDigest); supplied != "" && supplied != domain.DigestString(strings.TrimSpace(input.ContentExcerpt)) {
		return nil, domain.Invalid("content_digest", "提供的内容摘要与材料内容不一致")
	}
	now := s.clock().UTC()
	digest := domain.DigestString(strings.TrimSpace(input.ContentExcerpt))
	candidates, err := s.repo.FindByContentDigest(ctx, digest)
	if err != nil {
		return nil, err
	}
	related := make([]domain.DuplicateCaseSummary, 0, len(candidates))
	department := strings.TrimSpace(input.SourceDepartment)
	for _, candidate := range candidates {
		kind := "cross_department"
		if candidate.SourceDepartment == department {
			kind = "same_department"
		}
		related = append(related, domain.DuplicateCaseSummary{CaseID: candidate.ID, Status: candidate.Status,
			SourceDepartment: candidate.SourceDepartment, DuplicateKind: kind})
	}
	sort.Slice(related, func(i, j int) bool { return related[i].CaseID < related[j].CaseID })
	if err := domain.ValidateDuplicateAcceptance(input.AllowDuplicate, input.DuplicateReason, related); err != nil {
		return nil, err
	}
	id, err := s.newID("case_")
	if err != nil {
		return nil, err
	}
	c, err := domain.NewCase(id, input.Title, input.SourceDepartment, input.ContentExcerpt, meta.ActorID, now)
	if err != nil {
		return nil, err
	}
	if len(related) > 0 {
		c.DuplicateAcceptance = &domain.DuplicateAcceptanceEvidence{RelatedCases: related,
			ReasonDigest: domain.DigestString(strings.TrimSpace(input.DuplicateReason))}
	}
	event := s.event(c, "case.created", meta, "", struct {
		TitleDigest           string                        `json:"title_digest"`
		ContentDigest         string                        `json:"content_digest"`
		RelatedCases          []domain.DuplicateCaseSummary `json:"related_cases,omitempty"`
		DuplicateReasonDigest string                        `json:"duplicate_reason_digest,omitempty"`
	}{domain.DigestString(c.Title), c.ContentDigest, related,
		func() string {
			if c.DuplicateAcceptance != nil {
				return c.DuplicateAcceptance.ReasonDigest
			}
			return ""
		}()})
	if err := s.commit(ctx, operation, c, 0, event, meta.RequestID, true); err != nil {
		return nil, err
	}
	return c.Clone()
}
