package retrieval

import (
	"context"
	"fmt"
)

type Indexer struct {
	repository IndexRepository
	embedder   EmbeddingClient
	vectors    VectorIndex
	model      string
	cache      map[string][]float32
	gate       *WorkloadGate
}

func (indexer *Indexer) WithGate(gate *WorkloadGate) *Indexer { indexer.gate = gate; return indexer }

func NewIndexer(repository IndexRepository, embedder EmbeddingClient, vectors VectorIndex, model string) *Indexer {
	return &Indexer{repository: repository, embedder: embedder, vectors: vectors, model: model, cache: map[string][]float32{}}
}

func (indexer *Indexer) RunOnce(ctx context.Context, tenantID string, limit int) (IndexResult, error) {
	if indexer.gate != nil {
		indexer.gate.BeginIndex()
		defer indexer.gate.EndIndex()
	}
	synced, err := indexer.repository.SyncDocuments(ctx, tenantID, indexer.model, limit)
	if err != nil {
		return IndexResult{}, err
	}
	documents, err := indexer.repository.Pending(ctx, tenantID, indexer.model, limit)
	if err != nil || len(documents) == 0 {
		return IndexResult{Synced: synced}, err
	}
	ids := make([]string, len(documents))
	vectors := make([][]float32, len(documents))
	missingKeys := []string{}
	missingInputs := []string{}
	missingIndexes := map[string][]int{}
	for index, document := range documents {
		ids[index] = document.DocumentID
		if vector, ok := indexer.cache[document.ContentHash]; ok {
			vectors[index] = vector
			continue
		}
		if _, exists := missingIndexes[document.ContentHash]; !exists {
			missingKeys = append(missingKeys, document.ContentHash)
			missingInputs = append(missingInputs, EmbeddingInput(document))
		}
		missingIndexes[document.ContentHash] = append(missingIndexes[document.ContentHash], index)
	}
	embedded := [][]float32{}
	if len(missingInputs) > 0 {
		embedded, err = indexer.embedder.Embed(ctx, missingInputs)
	}
	if err != nil {
		_ = indexer.repository.MarkFailed(ctx, tenantID, ids, "embedding_failed")
		return IndexResult{Synced: synced}, err
	}
	if len(embedded) != len(missingInputs) {
		_ = indexer.repository.MarkFailed(ctx, tenantID, ids, "invalid_embeddings")
		return IndexResult{Synced: synced}, fmt.Errorf("invalid embeddings")
	}
	for keyIndex, key := range missingKeys {
		vector := embedded[keyIndex]
		if len(vector) == 0 {
			_ = indexer.repository.MarkFailed(ctx, tenantID, ids, "invalid_embeddings")
			return IndexResult{Synced: synced}, fmt.Errorf("invalid embeddings")
		}
		indexer.cache[key] = vector
		for _, documentIndex := range missingIndexes[key] {
			vectors[documentIndex] = vector
		}
	}
	if len(vectors) == 0 || len(vectors[0]) == 0 {
		return IndexResult{Synced: synced}, fmt.Errorf("invalid embeddings")
	}
	if err := indexer.vectors.EnsureCollection(ctx, len(vectors[0])); err != nil {
		_ = indexer.repository.MarkFailed(ctx, tenantID, ids, "vector_collection_failed")
		return IndexResult{Synced: synced}, err
	}
	points := make([]Point, len(documents))
	for index, document := range documents {
		points[index] = Point{ID: document.VectorKey, DocumentID: document.DocumentID, TenantID: document.TenantID, DeviceID: document.DeviceID, ProjectID: document.ProjectID, ActivityType: document.ActivityType, OccurredAt: document.OccurredAt, Vector: vectors[index]}
	}
	if err := indexer.vectors.Upsert(ctx, points); err != nil {
		_ = indexer.repository.MarkFailed(ctx, tenantID, ids, "vector_upsert_failed")
		return IndexResult{Synced: synced}, err
	}
	if err := indexer.repository.MarkIndexed(ctx, tenantID, ids); err != nil {
		return IndexResult{Synced: synced}, err
	}
	return IndexResult{Synced: synced, Indexed: len(documents)}, nil
}
