package domain

type ManifestRedaction struct {
	FindingID   string          `json:"finding_id"`
	Category    FindingCategory `json:"category"`
	Start       int             `json:"start"`
	End         int             `json:"end"`
	Decision    FindingDecision `json:"decision"`
	Replacement string          `json:"replacement,omitempty"`
}

type PublicationManifest struct {
	Version             int                 `json:"version"`
	CaseID              string              `json:"case_id"`
	Title               string              `json:"title"`
	SourceDepartment    string              `json:"source_department"`
	SourceContentDigest string              `json:"source_content_digest"`
	PublishedContent    string              `json:"published_content"`
	ContentFingerprint  string              `json:"content_fingerprint"`
	Redactions          []ManifestRedaction `json:"redactions"`
	ApprovedBy          string              `json:"approved_by"`
	ApprovalReason      string              `json:"approval_reason"`
	ManifestDigest      string              `json:"manifest_digest"`
}
