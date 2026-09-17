package processknowledge

import (
	"context"
	"regexp"
	"testing"

	"github.com/aetheris-dev/aetheris/server/internal/retrieval"
)

type fakeChunkRepository struct {
	pending []ChunkRecord
	indexed []string
}

func TestKnowledgeVectorKeyUsesQdrantUUID(t *testing.T) {
	key := knowledgeVectorKey("tenant", "chunk-1")
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`).MatchString(key) {
		t.Fatalf("invalid qdrant point id: %s", key)
	}
}

func TestVersionedChunkIDDiffersAcrossKnowledgeVersions(t *testing.T) {
	first := versionedChunkID("knowledge", 1, ChunkDraft{Index: 0, ContentHash: "same"})
	second := versionedChunkID("knowledge", 2, ChunkDraft{Index: 0, ContentHash: "same"})
	if first == second {
		t.Fatalf("chunk id reused across versions: %s", first)
	}
}

func (repository *fakeChunkRepository) PendingChunks(context.Context, string, string, int) ([]ChunkRecord, error) {
	return repository.pending, nil
}
func (repository *fakeChunkRepository) MarkChunksIndexed(_ context.Context, _ string, ids []string) error {
	repository.indexed = append(repository.indexed, ids...)
	return nil
}
func (repository *fakeChunkRepository) MarkChunksFailed(context.Context, string, []string, string) error {
	return nil
}

type fakeChunkEmbedder struct{ inputs []string }

func (embedder *fakeChunkEmbedder) Embed(_ context.Context, inputs []string) ([][]float32, error) {
	embedder.inputs = append(embedder.inputs, inputs...)
	return [][]float32{{0.1, 0.2}}, nil
}

type fakeChunkVectors struct{ points []retrieval.Point }

func (vectors *fakeChunkVectors) EnsureCollection(context.Context, int) error { return nil }
func (vectors *fakeChunkVectors) Upsert(_ context.Context, points []retrieval.Point) error {
	vectors.points = append(vectors.points, points...)
	return nil
}
func (vectors *fakeChunkVectors) Query(context.Context, []float32, retrieval.QueryFilter, int) ([]retrieval.Hit, error) {
	return nil, nil
}
func (vectors *fakeChunkVectors) Health(context.Context) error { return nil }

func TestKnowledgeIndexerEmbedsWholeChunkAndUsesLogicalProject(t *testing.T) {
	repository := &fakeChunkRepository{pending: []ChunkRecord{{ChunkID: "chunk-1", TenantID: "tenant", LogicalProjectID: "logical-safe", SessionID: "session", Topic: "EDR", SearchText: "完整过程知识 WFP", VectorKey: "vector-1", Version: 7}}}
	embedder := &fakeChunkEmbedder{}
	vectors := &fakeChunkVectors{}
	result, err := NewIndexer(repository, embedder, vectors, "embeddinggemma").RunOnce(context.Background(), "tenant", 10)
	if err != nil || result.Indexed != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if len(embedder.inputs) != 1 || embedder.inputs[0] != "完整过程知识 WFP" {
		t.Fatalf("inputs=%+v", embedder.inputs)
	}
	if len(vectors.points) != 1 || vectors.points[0].ProjectID != "logical-safe" || vectors.points[0].DocumentID != "chunk-1" || vectors.points[0].KnowledgeVersion != 7 {
		t.Fatalf("points=%+v", vectors.points)
	}
}
