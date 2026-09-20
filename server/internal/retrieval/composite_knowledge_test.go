package retrieval

import (
	"context"
	"errors"
	"testing"
)

type staticKnowledgeSearcher struct {
	hits    []KnowledgeHit
	err     error
	queries []KnowledgeQuery
}

func (searcher *staticKnowledgeSearcher) Search(_ context.Context, query KnowledgeQuery) ([]KnowledgeHit, error) {
	searcher.queries = append(searcher.queries, query)
	return searcher.hits, searcher.err
}

func TestCompositeKnowledgeRanksPrivateVerifiedBeforeCertifiedPublic(t *testing.T) {
	private := &staticKnowledgeSearcher{hits: []KnowledgeHit{
		{ChunkID: "private-unverified", KnowledgeID: "private-u", LogicalProjectID: "project-a", ValidationState: "unverified", Content: "未验证结论", Score: 0.9},
		{ChunkID: "private-verified", KnowledgeID: "private-v", LogicalProjectID: "project-b", ValidationState: "verified", Content: "已验证结论", Score: 0.7},
	}}
	public := &staticKnowledgeSearcher{hits: []KnowledgeHit{
		{ChunkID: "public", KnowledgeID: "public-1", PublicKnowledgeID: "public-1", SourceScope: "platform_public", ValidationState: "platform_certified", Content: "公共结论", Score: 0.8},
	}}
	hits, err := NewCompositeKnowledgeSearcher(private, public).Search(context.Background(), KnowledgeQuery{TenantID: "tenant-a", Question: "EDR", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 3 || hits[0].KnowledgeID != "private-v" || hits[1].KnowledgeID != "public-1" || hits[2].KnowledgeID != "private-u" {
		t.Fatalf("hits=%+v", hits)
	}
	if hits[0].SourceScope != "tenant_private" {
		t.Fatalf("private source scope=%q", hits[0].SourceScope)
	}
}

func TestCompositeKnowledgeSafelyDegradesWhenOneIndexFails(t *testing.T) {
	private := &staticKnowledgeSearcher{hits: []KnowledgeHit{{ChunkID: "private", KnowledgeID: "private", ValidationState: "verified", Content: "本租户结论"}}}
	public := &staticKnowledgeSearcher{err: errors.New("public unavailable")}
	hits, err := NewCompositeKnowledgeSearcher(private, public).Search(context.Background(), KnowledgeQuery{TenantID: "tenant-a", Question: "DLP", Limit: 5})
	if err != nil || len(hits) != 1 || hits[0].KnowledgeID != "private" {
		t.Fatalf("hits=%+v err=%v", hits, err)
	}
	if len(private.queries) != 1 || private.queries[0].TenantID != "tenant-a" {
		t.Fatalf("private query scope changed: %+v", private.queries)
	}
	private.err, private.hits = errors.New("private unavailable"), nil
	if _, err = NewCompositeKnowledgeSearcher(private, public).Search(context.Background(), KnowledgeQuery{TenantID: "tenant-a", Question: "DLP"}); err == nil {
		t.Fatal("both index failures must return an error")
	}
}

func TestCompositeKnowledgeShadowModeQueriesPublicButReturnsPrivateOnly(t *testing.T) {
	private := &staticKnowledgeSearcher{hits: []KnowledgeHit{{ChunkID: "private", KnowledgeID: "private", ValidationState: "verified", Content: "私有结论"}}}
	public := &staticKnowledgeSearcher{hits: []KnowledgeHit{{ChunkID: "public", KnowledgeID: "public", SourceScope: "platform_public", ValidationState: "platform_certified", Content: "公共结论"}}}
	hits, err := NewCompositeKnowledgeSearcher(private, public).WithMode("shadow").Search(context.Background(), KnowledgeQuery{TenantID: "tenant", Question: "EDR"})
	if err != nil || len(hits) != 1 || hits[0].KnowledgeID != "private" || len(public.queries) != 1 {
		t.Fatalf("hits=%+v public_queries=%d err=%v", hits, len(public.queries), err)
	}
}
