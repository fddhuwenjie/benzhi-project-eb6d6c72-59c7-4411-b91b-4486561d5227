package domain

import (
	"encoding/json"
	"time"
)

type AuditEvent struct {
	DataVersion   int             `json:"data_version"`
	Sequence      int64           `json:"sequence"`
	CaseID        string          `json:"case_id"`
	EventType     string          `json:"event_type"`
	ActorID       string          `json:"actor_id"`
	RequestID     string          `json:"request_id"`
	FromStatus    CaseStatus      `json:"from_status"`
	ToStatus      CaseStatus      `json:"to_status"`
	PayloadDigest string          `json:"payload_digest"`
	CaseRevision  int64           `json:"case_revision"`
	Payload       json.RawMessage `json:"payload,omitempty"`
	OccurredAt    time.Time       `json:"occurred_at"`
}

type TimelineEntry struct {
	Sequence   int64      `json:"sequence"`
	EventType  string     `json:"event_type"`
	ActorID    string     `json:"actor_id"`
	FromStatus CaseStatus `json:"from_status"`
	ToStatus   CaseStatus `json:"to_status"`
	OccurredAt time.Time  `json:"occurred_at"`
}
