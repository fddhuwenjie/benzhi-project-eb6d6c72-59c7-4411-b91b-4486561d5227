package domain

type CaseStatus string

const (
	StatusDraft             CaseStatus = "draft"
	StatusAwaitingRedaction CaseStatus = "awaiting_redaction"
	StatusAwaitingReview    CaseStatus = "awaiting_review"
	StatusChangesRequested  CaseStatus = "changes_requested"
	StatusApproved          CaseStatus = "approved"
	StatusPublished         CaseStatus = "published"
)

func (s CaseStatus) Valid() bool {
	switch s {
	case StatusDraft, StatusAwaitingRedaction, StatusAwaitingReview, StatusChangesRequested, StatusApproved, StatusPublished:
		return true
	default:
		return false
	}
}

func (s CaseStatus) Editable() bool {
	return s == StatusAwaitingRedaction || s == StatusChangesRequested
}

func CanTransition(from, to CaseStatus) bool {
	switch from {
	case StatusDraft:
		return to == StatusAwaitingRedaction
	case StatusAwaitingRedaction, StatusChangesRequested:
		return to == StatusAwaitingReview
	case StatusAwaitingReview:
		return to == StatusChangesRequested || to == StatusApproved
	case StatusApproved:
		return to == StatusPublished
	default:
		return false
	}
}
