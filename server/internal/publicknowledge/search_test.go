package publicknowledge

import (
	"context"
	"testing"

	"github.com/aetheris-dev/aetheris/server/internal/retrieval"
)

type fakeSearchRepository struct {
	vector  []SearchCandidate
	keyword []SearchCandidate
}

func (repository *fakeSearchRepository) LoadPublicVectorCandidates(context.Context, []retrieval.Hit) ([]SearchCandidate, error) {
	return repository.vector, nil
}
func (repository *fakeSearchRepository) PublicKeywordCandidates(context.Context, []string, int) ([]SearchCandidate, error) {
	return repository.keyword, nil
}

type fakeSearchVectors struct{ filter retrieval.QueryFilter }

func (vectors *fakeSearchVectors) EnsureCollection(context.Context, int) error     { return nil }
func (vectors *fakeSearchVectors) Upsert(context.Context, []retrieval.Point) error { return nil }
func (vectors *fakeSearchVectors) Delete(context.Context, []string) error          { return nil }
func (vectors *fakeSearchVectors) Query(_ context.Context, _ []float32, filter retrieval.QueryFilter, _ int) ([]retrieval.Hit, error) {
	vectors.filter = filter
	return []retrieval.Hit{{DocumentID: "chunk-1", Score: 0.9}}, nil
}
func (vectors *fakeSearchVectors) Health(context.Context) error { return nil }

func TestPublicSearcherUsesExplicitPublicScopeAndReturnsCertifiedKnowledge(t *testing.T) {
	repository := &fakeSearchRepository{
		vector:  []SearchCandidate{{ChunkID: "chunk-1", PublicKnowledgeID: "public-1", CanonicalTopic: "Windows EDR", ValidationState: string(PlatformCertified), Content: "内核采集保持轻量", Rank: 1}},
		keyword: []SearchCandidate{{ChunkID: "chunk-1", PublicKnowledgeID: "public-1", CanonicalTopic: "Windows EDR", ValidationState: string(PlatformCertified), Content: "内核采集保持轻量", Rank: 1}},
	}
	vectors := &fakeSearchVectors{}
	searcher := NewSearcher(repository, &fakePublicEmbedder{}, vectors)
	hits, err := searcher.Search(context.Background(), retrieval.KnowledgeQuery{TenantID: "tenant-c", Question: "Windows EDR 如何实现", Limit: 5})
	if err != nil || len(hits) != 1 {
		t.Fatalf("hits=%+v err=%v", hits, err)
	}
	if !vectors.filter.Public || vectors.filter.TenantID != "" || hits[0].SourceScope != "platform_public" || hits[0].PublicKnowledgeID != "public-1" {
		t.Fatalf("filter=%+v hit=%+v", vectors.filter, hits[0])
	}
}
