package retrieval

import (
	"context"
	"testing"
)

type fakeIndexRepository struct {
	documents []Document
	indexed   []string
}

func (repository *fakeIndexRepository) SyncDocuments(context.Context, string, string, int) (int, error) {
	return len(repository.documents), nil
}
func (repository *fakeIndexRepository) Pending(context.Context, string, string, int) ([]Document, error) {
	return repository.documents, nil
}
func (repository *fakeIndexRepository) MarkIndexed(_ context.Context, _ string, ids []string) error {
	repository.indexed = append(repository.indexed, ids...)
	return nil
}
func (repository *fakeIndexRepository) MarkFailed(context.Context, string, []string, string) error {
	return nil
}

type fakeEmbedder struct{ inputs int }

func (embedder *fakeEmbedder) Embed(_ context.Context, inputs []string) ([][]float32, error) {
	embedder.inputs += len(inputs)
	result := make([][]float32, len(inputs))
	for index := range result {
		result[index] = []float32{0.1, 0.2}
	}
	return result, nil
}

type fakeVectorIndex struct{ points []Point }

func (index *fakeVectorIndex) EnsureCollection(context.Context, int) error { return nil }
func (index *fakeVectorIndex) Upsert(_ context.Context, points []Point) error {
	index.points = append(index.points, points...)
	return nil
}
func (index *fakeVectorIndex) Query(context.Context, []float32, QueryFilter, int) ([]Hit, error) {
	return nil, nil
}
func (index *fakeVectorIndex) Health(context.Context) error { return nil }

func TestIndexerEmbedsUpsertsAndMarksBatch(t *testing.T) {
	repository := &fakeIndexRepository{documents: []Document{{DocumentID: "doc-1", TenantID: "tenant-1", Content: "内容一"}, {DocumentID: "doc-2", TenantID: "tenant-1", Content: "内容二"}}}
	vectors := &fakeVectorIndex{}
	embedder := &fakeEmbedder{}
	indexer := NewIndexer(repository, embedder, vectors, "bge-m3")

	result, err := indexer.RunOnce(context.Background(), "tenant-1", 10)

	if err != nil {
		t.Fatal(err)
	}
	if result.Indexed != 2 || len(vectors.points) != 2 || len(repository.indexed) != 2 {
		t.Fatalf("result=%#v points=%d indexed=%v", result, len(vectors.points), repository.indexed)
	}
	if embedder.inputs != 1 {
		t.Fatalf("duplicate content embedded %d times", embedder.inputs)
	}
}
