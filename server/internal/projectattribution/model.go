package projectattribution

import "time"

type Evidence struct {
	EventID         string
	DeviceID        string
	LegacyProjectID string
	SessionID       string
	Source          string
	ProjectLabel    string
}

type Candidate struct {
	LogicalProjectID string
	LocationID       string
}

type Candidates struct {
	Exact     []Candidate
	Inherited []Candidate
	Label     []Candidate
	Session   []Candidate
	Time      []Candidate
}

type Attribution struct {
	LogicalProjectID string         `json:"logical_project_id,omitempty"`
	LocationID       string         `json:"project_location_id,omitempty"`
	Method           string         `json:"method"`
	Confidence       string         `json:"confidence"`
	NeedsReview      bool           `json:"needs_review"`
	Evidence         map[string]any `json:"evidence"`
}

type Job struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenant_id"`
	Mode            string    `json:"mode"`
	RuleVersion     int       `json:"rule_version"`
	State           string    `json:"state"`
	ScannedCount    int64     `json:"scanned_count"`
	AssignedCount   int64     `json:"assigned_count"`
	UnresolvedCount int64     `json:"unresolved_count"`
	ConflictCount   int64     `json:"conflict_count"`
	LastEventID     string    `json:"last_event_id,omitempty"`
	ErrorCode       string    `json:"error_code,omitempty"`
	CreatedBy       string    `json:"created_by"`
	CreatedAt       time.Time `json:"created_at,omitempty"`
	UpdatedAt       time.Time `json:"updated_at,omitempty"`
	CompletedAt     time.Time `json:"completed_at,omitempty"`
}
