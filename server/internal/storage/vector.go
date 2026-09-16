package storage

import (
	"context"
	"fmt"
)

type Embedding struct{ TenantID, EventID, Provider, ModelVersion, VectorKey string }
type Hit struct {
	EventID string
	Score   float64
}

type VectorIndex interface {
	Upsert(context.Context, Embedding) error
	Search(context.Context, string, int) ([]Hit, error)
	Health(context.Context) error
}

type DisabledVectorIndex struct{}

func (DisabledVectorIndex) Upsert(context.Context, Embedding) error {
	return fmt.Errorf("vector index disabled")
}
func (DisabledVectorIndex) Search(context.Context, string, int) ([]Hit, error) {
	return nil, fmt.Errorf("vector index disabled")
}
func (DisabledVectorIndex) Health(context.Context) error { return nil }
