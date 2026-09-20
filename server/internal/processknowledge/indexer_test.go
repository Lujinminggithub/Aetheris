package processknowledge

import (
	"context"
	"regexp"
	"testing"
	"time"

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
func (vectors *fakeChunkVectors) Delete(context.Context, []string) error { return nil }
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

type signalingChunkEmbedder struct{ called chan struct{} }

func (embedder *signalingChunkEmbedder) Embed(context.Context, []string) ([][]float32, error) {
	close(embedder.called)
	return [][]float32{{0.1, 0.2}}, nil
}

func TestKnowledgeIndexerWaitsForForegroundQueryGate(t *testing.T) {
	repository := &fakeChunkRepository{pending: []ChunkRecord{{ChunkID: "chunk-1", SearchText: "EDR", VectorKey: "vector-1", Version: 8}}}
	embedder := &signalingChunkEmbedder{called: make(chan struct{})}
	gate := retrieval.NewWorkloadGate()
	gate.BeginQuery()
	done := make(chan error, 1)
	go func() {
		_, err := NewIndexer(repository, embedder, &fakeChunkVectors{}, "embeddinggemma").WithGate(gate).RunOnce(context.Background(), "tenant", 10)
		done <- err
	}()
	select {
	case <-embedder.called:
		t.Fatal("后台索引不应在前台查询持有锁时调用 embedding")
	case <-time.After(50 * time.Millisecond):
	}
	gate.EndQuery()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("释放前台查询锁后后台索引未恢复")
	}
}
