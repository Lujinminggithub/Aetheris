package retrieval

import (
	"context"
	"time"
)

type SourceFact struct {
	FactID, TenantID, SubjectID, DeviceID, ProjectID string
	RuleVersion                                      int
	EventType, Source, ActivityType, ActorOrigin     string
	MessageRole, AITool, CommandType                 string
	CommandSummary, CommandText, QualityState        string
	Excluded                                         bool
	OccurredAt                                       time.Time
	Payload                                          map[string]any
}

type Document struct {
	DocumentID, TenantID, FactID, SubjectID, DeviceID, ProjectID  string
	RuleVersion                                                   int
	ActivityType, Content, ContentHash, EmbeddingModel, VectorKey string
	OccurredAt                                                    time.Time
}

type Point struct {
	ID                string
	DocumentID        string
	TenantID          string
	DeviceID          string
	ProjectID         string
	ActivityType      string
	OccurredAt        time.Time
	KnowledgeVersion  int
	Public            bool
	PublicKnowledgeID string
	CanonicalTopic    string
	KnowledgeType     string
	ValidationState   string
	Revision          int
	Vector            []float32
}

type Hit struct {
	DocumentID string
	Score      float64
}

type QueryFilter struct {
	TenantID, DeviceID, ProjectID, ActivityType string
	From, ToExclusive                           time.Time
	KnowledgeVersion                            int
	Public                                      bool
}

type EmbeddingClient interface {
	Embed(context.Context, []string) ([][]float32, error)
}
type VectorIndex interface {
	EnsureCollection(context.Context, int) error
	Upsert(context.Context, []Point) error
	Delete(context.Context, []string) error
	Query(context.Context, []float32, QueryFilter, int) ([]Hit, error)
	Health(context.Context) error
}
type IndexRepository interface {
	SyncDocuments(context.Context, string, string, int) (int, error)
	Pending(context.Context, string, string, int) ([]Document, error)
	MarkIndexed(context.Context, string, []string) error
	MarkFailed(context.Context, string, []string, string) error
}

type IndexResult struct{ Synced, Indexed int }

type KnowledgeQuery struct {
	TenantID, LogicalProjectID, Question string
	From, ToExclusive                    time.Time
	Limit                                int
	AutoScope                            bool
}

type KnowledgeHit struct {
	ChunkID, KnowledgeID, SessionID, LogicalProjectID string
	PublicKnowledgeID, SourceScope                    string
	Topic, KnowledgeType, DecisionState               string
	ValidationState, Content, Applicability           string
	SourceEventIDs                                    []string
	Revision, AnonymousSourceTenantCount              int
	Score                                             float64
	OccurredAt                                        time.Time
}

type KnowledgeSearcher interface {
	Search(context.Context, KnowledgeQuery) ([]KnowledgeHit, error)
}
