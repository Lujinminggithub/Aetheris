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

func TestKeywordTermsWeightDomainAcronymsAboveGenericPlatformTerms(t *testing.T) {
	terms := KeywordTerms("如何在Windows实现DLP功能和OCR识别")
	counts := map[string]int{}
	for _, term := range terms {
		counts[term]++
	}
	if counts["dlp"] < 3 || counts["ocr"] < 3 || counts["windows"] != 1 {
		t.Fatalf("terms=%v counts=%v", terms, counts)
	}
}

func TestFilterDomainCandidatesPrefersDirectTopicMatches(t *testing.T) {
	candidates := []SearchCandidate{
		{ChunkID: "dlp-1", Topic: "DLP", Content: "OCR 与规则检测"},
		{ChunkID: "edr", Topic: "Windows EDR", Content: "EDR v2 IOCTL"},
		{ChunkID: "dlp-2", Topic: "DLP", Content: "剪贴板与文件阻断"},
	}
	result := FilterDomainCandidates(candidates, KeywordTerms("如何在Windows实现DLP功能"), 12)
	if len(result) != 2 || result[0].ChunkID != "dlp-1" || result[1].ChunkID != "dlp-2" {
		t.Fatalf("result=%+v", result)
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

func TestSelectAutomaticProjectUsesRepeatedRelevantKnowledge(t *testing.T) {
	candidates := []SearchCandidate{
		{LogicalProjectID: "logical-other", Rank: 1},
		{LogicalProjectID: "logical-safe", Rank: 2},
		{LogicalProjectID: "logical-safe", Rank: 3},
		{LogicalProjectID: "logical-safe", Rank: 4},
	}
	if got := SelectAutomaticProject(candidates); got != "logical-safe" {
		t.Fatalf("project=%q", got)
	}
}

func TestFuseCandidatesPrefersSubstantiveConclusionChunk(t *testing.T) {
	candidates := []SearchCandidate{
		{ChunkID: "tail", KnowledgeID: "knowledge-dlp", SessionID: "session-dlp", LogicalProjectID: "logical-safe", Content: "适用条件：Windows 终端。", Rank: 1},
		{ChunkID: "conclusion", KnowledgeID: "knowledge-dlp", SessionID: "session-dlp", LogicalProjectID: "logical-safe", Content: "主题：DLP\n结论：采集文件、剪贴板和截图内容，图片经 OCR 后进入规则引擎并执行记录、告警或阻断。", Rank: 2},
	}
	result := FuseCandidates(candidates, nil, 2)
	if len(result) != 1 || result[0].ChunkID != "conclusion" {
		t.Fatalf("result=%+v", result)
	}
}

func TestFuseCandidatesRemovesDuplicateContentAcrossKnowledgeIDs(t *testing.T) {
	content := "主题：DLP\n结论：图片经 OCR 后进入规则引擎并执行阻断。"
	result := FuseCandidates([]SearchCandidate{
		{ChunkID: "first", KnowledgeID: "knowledge-1", SessionID: "session-1", Content: content, Rank: 1},
		{ChunkID: "duplicate", KnowledgeID: "knowledge-2", SessionID: "session-2", Content: content, Rank: 2},
	}, nil, 10)
	if len(result) != 1 {
		t.Fatalf("result=%+v", result)
	}
}
