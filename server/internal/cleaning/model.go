package cleaning

import "time"

const CurrentRuleVersion = 3

type RawEvidence struct {
	EventID, TenantID, SubjectID, DeviceID, ProjectID, EventType, Source string
	OccurredAt, IngestedAt                                               time.Time
	Payload                                                              map[string]any
}

type Fact struct {
	FactID, TenantID, SubjectID, DeviceID, ProjectID        string
	RuleVersion                                             int
	OccurredAt, IngestedAt                                  time.Time
	FactType, EventType, Source, ActivityType               string
	ActorOrigin, MessageRole, AITool                        string
	CommandType, CommandSummary, CommandHash, CommandText   string
	QualityState, Confidence, MergeMethod, CanonicalEventID string
	ReasonCodes, SourceEventIDs                             []string
	ExcludedFromEffectiveness                               bool
}

type Job struct {
	ID               string    `json:"job_id"`
	DateFrom         string    `json:"date_from"`
	DateTo           string    `json:"date_to"`
	RuleVersion      int       `json:"rule_version"`
	Status           string    `json:"status"`
	ProcessedEvents  int       `json:"processed_events"`
	FactCount        int       `json:"fact_count"`
	MergedCount      int       `json:"merged_count"`
	QuarantinedCount int       `json:"quarantined_count"`
	ErrorCode        string    `json:"error_code,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

type Summary struct {
	RawEvents        int `json:"raw_events"`
	CleanFacts       int `json:"clean_facts"`
	MergedFacts      int `json:"merged_facts"`
	CommandFragments int `json:"command_fragments"`
	QuarantinedFacts int `json:"quarantined_facts"`
	ExcludedFacts    int `json:"excluded_facts"`
	RuleVersion      int `json:"rule_version"`
}

type FactView struct {
	FactID                    string    `json:"fact_id"`
	FactType                  string    `json:"fact_type"`
	EventType                 string    `json:"event_type"`
	Source                    string    `json:"source"`
	ActivityType              string    `json:"activity_type"`
	ActorOrigin               string    `json:"actor_origin"`
	ProjectID                 string    `json:"project_id"`
	ProjectName               string    `json:"project_name"`
	DeviceID                  string    `json:"device_id"`
	DeviceName                string    `json:"device_name"`
	QualityState              string    `json:"quality_state"`
	Confidence                string    `json:"confidence"`
	MergeMethod               string    `json:"merge_method"`
	OccurredAt                time.Time `json:"occurred_at"`
	ReasonCodes               []string  `json:"reason_codes"`
	SourceEventIDs            []string  `json:"source_event_ids"`
	CommandDisplay            string    `json:"command_display"`
	ExcludedFromEffectiveness bool      `json:"excluded_from_effectiveness"`
}

type FactPage struct {
	Facts  []FactView `json:"facts"`
	Total  int        `json:"total"`
	Limit  int        `json:"limit"`
	Offset int        `json:"offset"`
}
