package publicknowledge

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/aetheris-dev/aetheris/server/internal/retrieval"
)

func TestCrossTenantKnowledgeLifecycleKeepsPrivateSourcesHidden(t *testing.T) {
	base := PrivateKnowledge{
		KnowledgeID: "knowledge-a", Revision: 1, SessionID: "session-a", Topic: "Windows EDR", KnowledgeType: "implementation_pattern",
		Problem: "如何实现 Windows EDR", Conclusion: "内核采集保持轻量，复杂关联放在用户态。", Applicability: "Windows 11",
		DecisionState: "accepted", ValidationState: "verified", SourceTenantID: "tenant-a", SourceContentHash: "independent-a",
	}
	first, err := BuildCandidate(base)
	if err != nil {
		t.Fatal(err)
	}
	secondSource := base
	secondSource.SourceTenantID, secondSource.KnowledgeID, secondSource.SessionID, secondSource.SourceContentHash = "tenant-b", "knowledge-b", "session-b", "independent-b"
	second, err := BuildCandidate(secondSource)
	if err != nil {
		t.Fatal(err)
	}
	if first.Unit.ID != second.Unit.ID || first.Source.IndependenceGroup == second.Source.IndependenceGroup {
		t.Fatalf("candidate ids or independence groups are incorrect: first=%+v second=%+v", first, second)
	}
	raw, _ := json.Marshal(first.Unit)
	for _, forbidden := range []string{"tenant-a", "knowledge-a", "source_tenant_id", "source_knowledge_id"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("public unit leaks %s: %s", forbidden, raw)
		}
	}

	searchRepository := &fakeSearchRepository{vector: []SearchCandidate{{ChunkID: "chunk-1", PublicKnowledgeID: first.Unit.ID, CanonicalTopic: first.Unit.CanonicalTopic, Domains: first.Unit.Domains, ValidationState: string(EvidenceVerified), Content: first.Revision.Conclusion}}}
	searcher := NewSearcher(searchRepository, &fakePublicEmbedder{}, &fakeSearchVectors{})
	hits, err := searcher.Search(context.Background(), retrieval.KnowledgeQuery{TenantID: "tenant-c", Question: "Windows EDR", Limit: 5})
	if err != nil || len(hits) != 0 {
		t.Fatalf("uncertified knowledge was visible: hits=%+v err=%v", hits, err)
	}
	searchRepository.vector[0].ValidationState = string(PlatformCertified)
	hits, err = searcher.Search(context.Background(), retrieval.KnowledgeQuery{TenantID: "tenant-c", Question: "Windows EDR", Limit: 5})
	if err != nil || len(hits) != 1 || hits[0].SourceScope != "platform_public" || len(hits[0].SourceEventIDs) != 0 {
		t.Fatalf("certified public knowledge is unavailable or leaks evidence: hits=%+v err=%v", hits, err)
	}

	indexRepository := &fakeIndexRepository{withdrawals: []WithdrawalRecord{{ChunkID: "chunk-1", VectorKey: "point-1"}}}
	vectors := &fakePublicVectors{}
	result, err := NewIndexer(indexRepository, &fakePublicEmbedder{}, vectors, "embeddinggemma").RunOnce(context.Background(), 10)
	if err != nil || result.Deleted != 1 || len(vectors.deleted) != 1 {
		t.Fatalf("withdrawal was not removed: result=%+v deleted=%+v err=%v", result, vectors.deleted, err)
	}
}
