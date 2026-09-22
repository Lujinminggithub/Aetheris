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
		vector:  []SearchCandidate{{ChunkID: "chunk-1", PublicKnowledgeID: "public-1", CanonicalTopic: "Windows EDR", Domains: []string{"edr"}, ValidationState: string(PlatformCertified), Content: "内核采集保持轻量", Rank: 1}},
		keyword: []SearchCandidate{{ChunkID: "chunk-1", PublicKnowledgeID: "public-1", CanonicalTopic: "Windows EDR", Domains: []string{"edr"}, ValidationState: string(PlatformCertified), Content: "内核采集保持轻量", Rank: 1}},
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

func TestPublicSearcherExcludesCrossDomainCertifiedKnowledge(t *testing.T) {
	repository := &fakeSearchRepository{vector: []SearchCandidate{
		{ChunkID: "dlp", PublicKnowledgeID: "public-dlp", CanonicalTopic: "DLP", Domains: []string{"dlp"}, ValidationState: string(PlatformCertified), Content: "DLP OCR 策略阻断", Rank: 2},
		{ChunkID: "fec", PublicKnowledgeID: "public-fec", CanonicalTopic: "网络传输", Domains: []string{"network_transport"}, ValidationState: string(PlatformCertified), Content: "FEC 公网丢包媒体队列", Rank: 1},
	}}
	searcher := NewSearcher(repository, &fakePublicEmbedder{}, &fakeSearchVectors{})
	hits, err := searcher.Search(context.Background(), retrieval.KnowledgeQuery{TenantID: "tenant", Question: "Windows DLP 的实现原理", Limit: 5})
	if err != nil || len(hits) != 1 || hits[0].PublicKnowledgeID != "public-dlp" {
		t.Fatalf("hits=%+v err=%v", hits, err)
	}
}
