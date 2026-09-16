package events

import "encoding/json"

type RoleSnapshot struct {
	RoleID  string `json:"role_id"`
	Code    string `json:"code"`
	Version int    `json:"version"`
	Source  string `json:"source"`
}

type Event struct {
	EventID           string         `json:"event_id"`
	SchemaVersion     int            `json:"schema_version"`
	EventType         string         `json:"event_type"`
	TenantID          string         `json:"tenant_id"`
	SubjectID         string         `json:"subject_id"`
	DeviceID          string         `json:"device_id"`
	ProjectID         string         `json:"project_id"`
	SessionID         string         `json:"session_id"`
	CorrelationID     string         `json:"correlation_id"`
	Source            string         `json:"source"`
	SourceVersion     string         `json:"source_version"`
	OccurredAt        string         `json:"occurred_at"`
	IngestedAt        string         `json:"ingested_at"`
	Payload           map[string]any `json:"payload"`
	ContentRefs       []any          `json:"content_refs,omitempty"`
	ContentHash       string         `json:"content_hash"`
	Provenance        map[string]any `json:"provenance,omitempty"`
	RedactionReport   map[string]any `json:"redaction_report"`
	ProcessingGrants  []string       `json:"processing_grants"`
	CryptoMode        string         `json:"crypto_mode,omitempty"`
	KeyVersion        *string        `json:"key_version,omitempty"`
	SupersedesEventID *string        `json:"supersedes_event_id,omitempty"`
	WorkRole          *RoleSnapshot  `json:"work_role,omitempty"`
	Raw               map[string]any `json:"-"`
}

func (event Event) JSON() ([]byte, error) {
	return json.Marshal(event.Raw)
}
