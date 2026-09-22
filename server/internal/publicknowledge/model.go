package publicknowledge

import "time"

type PublicationState string
type ValidationState string
type ScopeState string

const (
	CandidateState PublicationState = "candidate"
	PendingReview  PublicationState = "pending_review"
	Published      PublicationState = "published"
	Suspended      PublicationState = "suspended"
	Withdrawn      PublicationState = "withdrawn"

	Unverified              ValidationState = "unverified"
	SourceConfirmed         ValidationState = "source_confirmed"
	EvidenceVerified        ValidationState = "evidence_verified"
	CrossTenantCorroborated ValidationState = "cross_tenant_corroborated"
	PlatformCertified       ValidationState = "platform_certified"
	Contradicted            ValidationState = "contradicted"

	ScopeClassified   ScopeState = "classified"
	ScopeUnclassified ScopeState = "unclassified"
	ScopeRejected     ScopeState = "rejected"
)

type Revision struct {
	Revision                   int             `json:"revision"`
	ProblemPattern             string          `json:"problem_pattern"`
	Conclusion                 string          `json:"conclusion"`
	Rationale                  string          `json:"rationale"`
	Applicability              string          `json:"applicability"`
	Caveats                    string          `json:"caveats"`
	Alternatives               string          `json:"alternatives"`
	ValidationState            ValidationState `json:"validation_state"`
	AnonymousSourceTenantCount int             `json:"anonymous_source_tenant_count"`
	IndependentSessionCount    int             `json:"independent_session_count"`
	CanonicalHash              string          `json:"canonical_hash"`
	ExtractorVersion           string          `json:"extractor_version"`
	RedactionVersion           string          `json:"redaction_version"`
	ReviewPolicyVersion        string          `json:"review_policy_version"`
	CreatedAt                  time.Time       `json:"created_at"`
}

type Unit struct {
	ID               string               `json:"public_knowledge_id"`
	CanonicalTopic   string               `json:"canonical_topic"`
	KnowledgeType    string               `json:"knowledge_type"`
	PublicationState PublicationState     `json:"publication_state"`
	CurrentRevision  int                  `json:"current_revision"`
	Domains          []string             `json:"domains"`
	Entities         []string             `json:"entities"`
	ScopeState       ScopeState           `json:"scope_state"`
	Revision         Revision             `json:"current"`
	Reviews          []Review             `json:"reviews,omitempty"`
	PrivateEvidence  []PrivateEvidenceRef `json:"private_evidence,omitempty"`
	CreatedAt        time.Time            `json:"created_at"`
	UpdatedAt        time.Time            `json:"updated_at"`
}

type PrivateEvidenceRef struct {
	KnowledgeID string `json:"knowledge_id"`
	Revision    int    `json:"revision"`
	Relation    string `json:"relation"`
}

type SourceLink struct {
	ID                 string
	PublicKnowledgeID  string
	PublicRevision     int
	SourceTenantID     string
	SourceKnowledgeID  string
	SourceRevision     int
	Relation           string
	IndependenceGroup  string
	SourceContentHash  string
	ExternalSourceHash string
	ImportBatchID      string
	RedactionState     string
}

type Review struct {
	ID        string    `json:"review_id"`
	Revision  int       `json:"revision"`
	Action    string    `json:"action"`
	ActorID   string    `json:"actor_id"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

type ListFilter struct {
	PublicationState string
	ValidationState  string
	Topic            string
	ViewerTenantID   string
	Limit            int
	Offset           int
}

type ReviewCommand struct {
	KnowledgeID      string `json:"-"`
	ExpectedRevision int    `json:"expected_revision"`
	ActorID          string `json:"-"`
	ActorTenantID    string `json:"-"`
	Action           string `json:"-"`
	Reason           string `json:"reason"`
}

type Transition struct {
	PublicationState PublicationState
	ValidationState  ValidationState
}

type Job struct {
	ID              string     `json:"id"`
	Mode            string     `json:"mode"`
	State           string     `json:"state"`
	LastTenantID    string     `json:"last_tenant_id,omitempty"`
	LastKnowledgeID string     `json:"last_knowledge_id,omitempty"`
	ScannedCount    int64      `json:"scanned_count"`
	CandidateCount  int64      `json:"candidate_count"`
	ConflictCount   int64      `json:"conflict_count"`
	FailedCount     int64      `json:"failed_count"`
	ErrorCode       string     `json:"error_code,omitempty"`
	CreatedBy       string     `json:"created_by"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
}

type PrivateKnowledge struct {
	SourceTenantID, KnowledgeID, SessionID, Topic, KnowledgeType string
	Problem, Conclusion, Rationale, Applicability, Caveats       string
	Alternatives, DecisionState, ValidationState                 string
	SourceContentHash, ExternalSourceHash, ImportBatchID         string
	Revision                                                     int
	SensitiveTerms                                               []string
}

type Candidate struct {
	Unit     Unit
	Revision Revision
	Source   SourceLink
}

type Summary struct {
	Candidate  int `json:"candidate"`
	Pending    int `json:"pending_review"`
	Published  int `json:"published"`
	Suspended  int `json:"suspended"`
	Withdrawn  int `json:"withdrawn"`
	Conflicts  int `json:"open_conflicts"`
	ActiveJobs int `json:"active_jobs"`
}
