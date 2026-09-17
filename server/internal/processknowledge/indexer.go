package processknowledge

import (
	"context"
	"fmt"
	"time"

	"github.com/aetheris-dev/aetheris/server/internal/retrieval"
)

type ChunkRecord struct {
	ChunkID, TenantID, LogicalProjectID, SessionID, Topic string
	SearchText, VectorKey                                 string
	OccurredAt                                            time.Time
	Version                                               int
}

type ChunkRepository interface {
	PendingChunks(context.Context, string, string, int) ([]ChunkRecord, error)
	MarkChunksIndexed(context.Context, string, []string) error
	MarkChunksFailed(context.Context, string, []string, string) error
}

type IndexResult struct{ Indexed int }

func knowledgeVectorKey(tenantID, chunkID string) string { return retrieval.PointID(tenantID, chunkID) }
func versionedChunkID(knowledgeID string, version int, chunk ChunkDraft) string {
	return stableID("chunk", knowledgeID, fmt.Sprint(version), fmt.Sprint(chunk.Index), chunk.ContentHash)
}

type Indexer struct {
	repository ChunkRepository
	embedder   retrieval.EmbeddingClient
	vectors    retrieval.VectorIndex
	model      string
	gate       *retrieval.WorkloadGate
}

func NewIndexer(repository ChunkRepository, embedder retrieval.EmbeddingClient, vectors retrieval.VectorIndex, model string) *Indexer {
	return &Indexer{repository: repository, embedder: embedder, vectors: vectors, model: model}
}

func (indexer *Indexer) WithGate(gate *retrieval.WorkloadGate) *Indexer {
	indexer.gate = gate
	return indexer
}

func (indexer *Indexer) RunOnce(ctx context.Context, tenantID string, limit int) (IndexResult, error) {
	if indexer.gate != nil {
		indexer.gate.BeginIndex()
		defer indexer.gate.EndIndex()
	}
	chunks, err := indexer.repository.PendingChunks(ctx, tenantID, indexer.model, limit)
	if err != nil || len(chunks) == 0 {
		return IndexResult{}, err
	}
	inputs := make([]string, len(chunks))
	ids := make([]string, len(chunks))
	for index, chunk := range chunks {
		inputs[index], ids[index] = chunk.SearchText, chunk.ChunkID
	}
	vectors, err := indexer.embedder.Embed(ctx, inputs)
	if err != nil || len(vectors) != len(chunks) {
		_ = indexer.repository.MarkChunksFailed(ctx, tenantID, ids, "embedding_failed")
		if err == nil {
			err = fmt.Errorf("embedding count mismatch")
		}
		return IndexResult{}, err
	}
	if len(vectors[0]) == 0 {
		return IndexResult{}, fmt.Errorf("empty embedding")
	}
	if err := indexer.vectors.EnsureCollection(ctx, len(vectors[0])); err != nil {
		return IndexResult{}, err
	}
	points := make([]retrieval.Point, len(chunks))
	for index, chunk := range chunks {
		points[index] = retrieval.Point{ID: chunk.VectorKey, DocumentID: chunk.ChunkID, TenantID: tenantID, ProjectID: chunk.LogicalProjectID, ActivityType: "process_knowledge", OccurredAt: chunk.OccurredAt, KnowledgeVersion: chunk.Version, Vector: vectors[index]}
	}
	if err := indexer.vectors.Upsert(ctx, points); err != nil {
		_ = indexer.repository.MarkChunksFailed(ctx, tenantID, ids, "vector_upsert_failed")
		return IndexResult{}, err
	}
	if err := indexer.repository.MarkChunksIndexed(ctx, tenantID, ids); err != nil {
		return IndexResult{}, err
	}
	return IndexResult{Indexed: len(chunks)}, nil
}

type IndexWorker struct {
	indexer  *Indexer
	tenantID string
	interval time.Duration
	batch    int
}

func NewIndexWorker(indexer *Indexer, tenantID string, interval time.Duration, batch int) *IndexWorker {
	return &IndexWorker{indexer: indexer, tenantID: tenantID, interval: interval, batch: batch}
}
func (worker *IndexWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(worker.interval)
	defer ticker.Stop()
	for {
		_, _ = worker.indexer.RunOnce(ctx, worker.tenantID, worker.batch)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
