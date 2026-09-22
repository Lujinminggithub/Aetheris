package publicknowledge

import (
	"context"
	"fmt"
	"time"

	"github.com/aetheris-dev/aetheris/server/internal/retrieval"
)

type ChunkRecord struct {
	ChunkID, PublicKnowledgeID, CanonicalTopic, KnowledgeType string
	ValidationState, SearchText, VectorKey                    string
	Domains, Entities                                         []string
	Revision                                                  int
}

type IndexWorker struct {
	indexer  *Indexer
	interval time.Duration
	batch    int
}

func NewIndexWorker(indexer *Indexer, interval time.Duration, batch int) *IndexWorker {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	if batch < 1 {
		batch = 25
	}
	return &IndexWorker{indexer: indexer, interval: interval, batch: batch}
}

func (worker *IndexWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(worker.interval)
	defer ticker.Stop()
	for {
		_, _ = worker.indexer.RunOnce(ctx, worker.batch)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

type WithdrawalRecord struct {
	ChunkID, VectorKey string
}

type IndexRepository interface {
	PendingPublicChunks(context.Context, string, int) ([]ChunkRecord, error)
	PendingWithdrawals(context.Context, int) ([]WithdrawalRecord, error)
	MarkPublicChunksIndexed(context.Context, []string) error
	MarkPublicChunksFailed(context.Context, []string, string) error
	MarkWithdrawalsDeleted(context.Context, []string) error
}

type IndexResult struct {
	Indexed int
	Deleted int
}

type Indexer struct {
	repository IndexRepository
	embedder   retrieval.EmbeddingClient
	vectors    retrieval.VectorIndex
	model      string
	gate       *retrieval.WorkloadGate
}

func NewIndexer(repository IndexRepository, embedder retrieval.EmbeddingClient, vectors retrieval.VectorIndex, model string) *Indexer {
	return &Indexer{repository: repository, embedder: embedder, vectors: vectors, model: model}
}

func (indexer *Indexer) WithGate(gate *retrieval.WorkloadGate) *Indexer {
	indexer.gate = gate
	return indexer
}

func (indexer *Indexer) RunOnce(ctx context.Context, limit int) (IndexResult, error) {
	if limit < 1 {
		limit = 25
	}
	if indexer.gate != nil {
		indexer.gate.BeginIndex()
		defer indexer.gate.EndIndex()
	}
	result := IndexResult{}
	withdrawals, err := indexer.repository.PendingWithdrawals(ctx, limit)
	if err != nil {
		return result, err
	}
	if len(withdrawals) > 0 {
		keys, ids := make([]string, len(withdrawals)), make([]string, len(withdrawals))
		for index, item := range withdrawals {
			keys[index], ids[index] = item.VectorKey, item.ChunkID
		}
		if err = indexer.vectors.Delete(ctx, keys); err != nil {
			return result, err
		}
		if err = indexer.repository.MarkWithdrawalsDeleted(ctx, ids); err != nil {
			return result, err
		}
		result.Deleted = len(ids)
		return result, nil
	}
	chunks, err := indexer.repository.PendingPublicChunks(ctx, indexer.model, limit)
	if err != nil || len(chunks) == 0 {
		return result, err
	}
	inputs, ids := make([]string, len(chunks)), make([]string, len(chunks))
	for index, chunk := range chunks {
		inputs[index], ids[index] = chunk.SearchText, chunk.ChunkID
	}
	vectors, err := indexer.embedder.Embed(ctx, inputs)
	if err != nil || len(vectors) != len(chunks) {
		_ = indexer.repository.MarkPublicChunksFailed(ctx, ids, "embedding_failed")
		if err == nil {
			err = fmt.Errorf("embedding count mismatch")
		}
		return result, err
	}
	if len(vectors) == 0 || len(vectors[0]) == 0 {
		return result, fmt.Errorf("empty embedding")
	}
	if err = indexer.vectors.EnsureCollection(ctx, len(vectors[0])); err != nil {
		_ = indexer.repository.MarkPublicChunksFailed(ctx, ids, "vector_collection_failed")
		return result, err
	}
	points := make([]retrieval.Point, len(chunks))
	for index, chunk := range chunks {
		points[index] = retrieval.Point{
			ID: chunk.VectorKey, DocumentID: chunk.ChunkID, Public: true,
			PublicKnowledgeID: chunk.PublicKnowledgeID, CanonicalTopic: chunk.CanonicalTopic,
			KnowledgeType: chunk.KnowledgeType, ValidationState: chunk.ValidationState,
			Domains: chunk.Domains, Entities: chunk.Entities,
			Revision: chunk.Revision, ActivityType: "public_knowledge", Vector: vectors[index],
		}
	}
	if err = indexer.vectors.Upsert(ctx, points); err != nil {
		_ = indexer.repository.MarkPublicChunksFailed(ctx, ids, "vector_upsert_failed")
		return result, err
	}
	if err = indexer.repository.MarkPublicChunksIndexed(ctx, ids); err != nil {
		return result, err
	}
	result.Indexed = len(chunks)
	return result, nil
}
