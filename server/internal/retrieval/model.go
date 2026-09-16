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
	ID           string
	DocumentID   string
	TenantID     string
	DeviceID     string
	ProjectID    string
	ActivityType string
	OccurredAt   time.Time
	Vector       []float32
}

type Hit struct {
	DocumentID string
	Score      float64
}

type QueryFilter struct {
	TenantID, DeviceID, ProjectID, ActivityType string
	From, ToExclusive                           time.Time
}

type EmbeddingClient interface {
	Embed(context.Context, []string) ([][]float32, error)
}
type VectorIndex interface {
	EnsureCollection(context.Context, int) error
	Upsert(context.Context, []Point) error
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
