package processknowledge

import "time"

type StatementKind string

const (
	HumanQuestion     StatementKind = "human_question"
	HumanConstraint   StatementKind = "human_constraint"
	HumanFollowup     StatementKind = "human_followup"
	HumanConfirmation StatementKind = "human_confirmation"
	HumanRejection    StatementKind = "human_rejection"
	AIExploration     StatementKind = "ai_exploration"
	AIFinalAnswer     StatementKind = "ai_final_answer"
	ToolCall          StatementKind = "tool_call"
	ToolResult        StatementKind = "tool_result"
	CodeChange        StatementKind = "code_change"
	TestResult        StatementKind = "test_result"
	BuildResult       StatementKind = "build_result"
	RuntimeValidation StatementKind = "runtime_validation"
	SystemContext     StatementKind = "system_context"
)

type SourceTurn struct {
	TenantID, SubjectID, DeviceID, LogicalProjectID string
	AITool, SessionID, ParentMessageID              string
	EventID, FactID, Role, EventType, Content       string
	OccurredAt                                      time.Time
}

type TurnDraft struct {
	ID       string
	Sequence int
	Kind     StatementKind
	Source   SourceTurn
}

type SessionDraft struct {
	ID, TenantID, SubjectID, DeviceID, LogicalProjectID string
	AITool, SourceSessionID                             string
	AssociationMethod, AssociationConfidence            string
	StartedAt, EndedAt                                  time.Time
	Turns                                               []TurnDraft
}

type EvidenceDraft struct {
	ID         string `json:"evidence_id"`
	Kind       string `json:"evidence_kind"`
	EventID    string `json:"event_id,omitempty"`
	FactID     string `json:"fact_id,omitempty"`
	TurnID     string `json:"turn_id,omitempty"`
	Section    string `json:"supports_section"`
	Relation   string `json:"relation"`
	ReasonCode string `json:"reason_code"`
}

type KnowledgeDraft struct {
	ID               string          `json:"knowledge_id"`
	SessionID        string          `json:"session_id"`
	LogicalProjectID string          `json:"logical_project_id"`
	Topic            string          `json:"topic"`
	KnowledgeType    string          `json:"knowledge_type"`
	Problem          string          `json:"problem"`
	Intent           string          `json:"intent"`
	Constraints      string          `json:"constraints"`
	Conclusion       string          `json:"conclusion"`
	Rationale        string          `json:"rationale"`
	Alternatives     string          `json:"alternatives"`
	Applicability    string          `json:"applicability"`
	Caveats          string          `json:"caveats"`
	DecisionState    string          `json:"decision_state"`
	ValidationState  string          `json:"validation_state"`
	LifecycleState   string          `json:"lifecycle_state"`
	OccurredAt       time.Time       `json:"occurred_at"`
	Evidence         []EvidenceDraft `json:"evidence,omitempty"`
}

type ChunkDraft struct {
	ID, KnowledgeID, Topic, Content, SearchText, ContentHash string
	Index                                                    int
}

type Job struct {
	ID               string     `json:"id"`
	TenantID         string     `json:"tenant_id"`
	Mode             string     `json:"mode"`
	LogicalProjectID string     `json:"logical_project_id,omitempty"`
	State            string     `json:"state"`
	LastSessionID    string     `json:"last_session_id,omitempty"`
	CreatedBy        string     `json:"created_by"`
	ErrorCode        string     `json:"error_code,omitempty"`
	Version          int        `json:"version"`
	ScannedCount     int64      `json:"scanned_count"`
	CandidateCount   int64      `json:"candidate_count"`
	VerifiedCount    int64      `json:"verified_count"`
	ConflictCount    int64      `json:"conflict_count"`
	FailedCount      int64      `json:"failed_count"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
}

type Summary struct {
	Sessions      int    `json:"sessions"`
	Units         int    `json:"units"`
	Verified      int    `json:"verified"`
	Conflicts     int    `json:"conflicts"`
	Unattributed  int    `json:"unattributed"`
	Chunks        int    `json:"chunks"`
	ActiveVersion int    `json:"active_version"`
	Mode          string `json:"mode"`
}

type ListFilter struct {
	TenantID, LogicalProjectID, Topic, ValidationState, DecisionState string
	Limit, Offset                                                     int
}

type KnowledgeUnit struct {
	KnowledgeDraft
	Revision int             `json:"revision"`
	Version  int             `json:"version"`
	Evidence []EvidenceDraft `json:"evidence"`
}
