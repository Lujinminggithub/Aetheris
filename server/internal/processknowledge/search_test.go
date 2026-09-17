package processknowledge

import "testing"

func TestFuseCandidatesPrioritizesVerifiedAndDeduplicatesKnowledgeAndSession(t *testing.T) {
	vector := []SearchCandidate{
		{ChunkID: "a1", KnowledgeID: "knowledge-a", SessionID: "session-a", ValidationState: "unverified", DecisionState: "proposed", Rank: 1},
		{ChunkID: "b1", KnowledgeID: "knowledge-b", SessionID: "session-b", ValidationState: "verified", DecisionState: "accepted", Rank: 2},
		{ChunkID: "a2", KnowledgeID: "knowledge-a", SessionID: "session-a", ValidationState: "unverified", DecisionState: "proposed", Rank: 3},
	}
	keyword := []SearchCandidate{
		{ChunkID: "b1", KnowledgeID: "knowledge-b", SessionID: "session-b", ValidationState: "verified", DecisionState: "accepted", Rank: 1},
		{ChunkID: "c1", KnowledgeID: "knowledge-c", SessionID: "session-b", ValidationState: "partially_verified", DecisionState: "accepted", Rank: 2},
	}
	result := FuseCandidates(vector, keyword, 10)
	if len(result) != 3 {
		t.Fatalf("result=%+v", result)
	}
	if result[0].KnowledgeID != "knowledge-b" {
		t.Fatalf("unexpected ranking: %+v", result)
	}
}

func TestKeywordTermsKeepWindowsAPIIdentifiers(t *testing.T) {
	terms := KeywordTerms("Windows EDR 如何使用 PsSetCreateProcessNotifyRoutineEx 和 WFP？")
	want := map[string]bool{"windows": true, "edr": true, "pssetcreateprocessnotifyroutineex": true, "wfp": true}
	for _, term := range terms {
		delete(want, term)
	}
	if len(want) != 0 {
		t.Fatalf("missing terms: %+v", want)
	}
}

func TestFuseCandidatesKeepsDualChannelRelevanceAheadOfStatusOnlyHit(t *testing.T) {
	statusOnly := SearchCandidate{ChunkID: "status", KnowledgeID: "knowledge-status", SessionID: "session-status", ValidationState: "verified", DecisionState: "accepted", Rank: 1}
	relevant := SearchCandidate{ChunkID: "edr", KnowledgeID: "knowledge-edr", SessionID: "session-edr", ValidationState: "unverified", DecisionState: "proposed", Rank: 2}
	result := FuseCandidates([]SearchCandidate{statusOnly, relevant}, []SearchCandidate{{ChunkID: "edr", KnowledgeID: "knowledge-edr", SessionID: "session-edr", ValidationState: "unverified", DecisionState: "proposed", Rank: 1}}, 2)
	if len(result) != 2 || result[0].ChunkID != "edr" {
		t.Fatalf("relevance was overridden by status bonus: %+v", result)
	}
}
