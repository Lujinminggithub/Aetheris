package publicknowledge

import (
	"context"
	"testing"

	"github.com/aetheris-dev/aetheris/server/internal/retrieval"
)

type fakeIndexRepository struct {
	pending     []ChunkRecord
	withdrawals []WithdrawalRecord
	indexed     []string
	deleted     []string
}

func (repository *fakeIndexRepository) PendingPublicChunks(context.Context, string, int) ([]ChunkRecord, error) {
	return repository.pending, nil
}
func (repository *fakeIndexRepository) PendingWithdrawals(context.Context, int) ([]WithdrawalRecord, error) {
	return repository.withdrawals, nil
}
func (repository *fakeIndexRepository) MarkPublicChunksIndexed(_ context.Context, ids []string) error {
	repository.indexed = append(repository.indexed, ids...)
	return nil
}
func (repository *fakeIndexRepository) MarkPublicChunksFailed(context.Context, []string, string) error {
	return nil
}
func (repository *fakeIndexRepository) MarkWithdrawalsDeleted(_ context.Context, ids []string) error {
	repository.deleted = append(repository.deleted, ids...)
	return nil
}

type fakePublicEmbedder struct{ inputs []string }

func (embedder *fakePublicEmbedder) Embed(_ context.Context, inputs []string) ([][]float32, error) {
	embedder.inputs = append(embedder.inputs, inputs...)
	vectors := make([][]float32, len(inputs))
	for index := range vectors {
		vectors[index] = []float32{0.1, 0.2}
	}
	return vectors, nil
}

type fakePublicVectors struct {
	points  []retrieval.Point
	deleted []string
}

func (vectors *fakePublicVectors) EnsureCollection(context.Context, int) error { return nil }
func (vectors *fakePublicVectors) Upsert(_ context.Context, points []retrieval.Point) error {
	vectors.points = append(vectors.points, points...)
	return nil
}
func (vectors *fakePublicVectors) Delete(_ context.Context, ids []string) error {
	vectors.deleted = append(vectors.deleted, ids...)
	return nil
}
func (vectors *fakePublicVectors) Query(context.Context, []float32, retrieval.QueryFilter, int) ([]retrieval.Hit, error) {
	return nil, nil
}
func (vectors *fakePublicVectors) Health(context.Context) error { return nil }

func TestPublicIndexerWritesOnlyPublicPayloadAndPublishesAfterIndex(t *testing.T) {
	repository := &fakeIndexRepository{pending: []ChunkRecord{{
		ChunkID: "chunk-1", PublicKnowledgeID: "public-1", CanonicalTopic: "Windows EDR",
		KnowledgeType: "implementation_pattern", ValidationState: string(PlatformCertified), Revision: 2,
		SearchText: "Windows EDR 内核采集与用户态分析", VectorKey: "point-1",
	}}}
	embedder := &fakePublicEmbedder{}
	vectors := &fakePublicVectors{}
	result, err := NewIndexer(repository, embedder, vectors, "embeddinggemma").RunOnce(context.Background(), 10)
	if err != nil || result.Indexed != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if len(vectors.points) != 1 || !vectors.points[0].Public || vectors.points[0].TenantID != "" || vectors.points[0].ProjectID != "" || vectors.points[0].PublicKnowledgeID != "public-1" {
		t.Fatalf("points=%+v", vectors.points)
	}
	if len(repository.indexed) != 1 || repository.indexed[0] != "chunk-1" {
		t.Fatalf("indexed=%v", repository.indexed)
	}
}

func TestPublicIndexerDeletesWithdrawnVectorsBeforeNewIndexing(t *testing.T) {
	repository := &fakeIndexRepository{withdrawals: []WithdrawalRecord{{ChunkID: "chunk-old", VectorKey: "point-old"}}}
	vectors := &fakePublicVectors{}
	result, err := NewIndexer(repository, &fakePublicEmbedder{}, vectors, "embeddinggemma").RunOnce(context.Background(), 10)
	if err != nil || result.Deleted != 1 || len(vectors.deleted) != 1 || vectors.deleted[0] != "point-old" {
		t.Fatalf("result=%+v vectors=%+v err=%v", result, vectors.deleted, err)
	}
	if len(repository.deleted) != 1 || repository.deleted[0] != "chunk-old" {
		t.Fatalf("marked=%v", repository.deleted)
	}
}
