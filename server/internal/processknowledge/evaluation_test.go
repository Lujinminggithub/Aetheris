package processknowledge

import (
	"testing"

	"github.com/aetheris-dev/aetheris/server/internal/retrieval"
)

func TestTrainingCandidateRequiresAcceptedVerifiedActiveKnowledge(t *testing.T) {
	valid := KnowledgeUnit{KnowledgeDraft: KnowledgeDraft{DecisionState: "accepted", ValidationState: "verified", LifecycleState: "active"}}
	if !EligibleTrainingCandidate(valid) {
		t.Fatal("verified active knowledge rejected")
	}
	for _, unit := range []KnowledgeUnit{
		{KnowledgeDraft: KnowledgeDraft{DecisionState: "proposed", ValidationState: "verified", LifecycleState: "active"}},
		{KnowledgeDraft: KnowledgeDraft{DecisionState: "accepted", ValidationState: "unverified", LifecycleState: "active"}},
		{KnowledgeDraft: KnowledgeDraft{DecisionState: "accepted", ValidationState: "verified", LifecycleState: "withdrawn"}},
	} {
		if EligibleTrainingCandidate(unit) {
			t.Fatalf("invalid candidate accepted: %+v", unit)
		}
	}
}

func TestEDRAcceptanceRequiresArchitectureCoverageAndKnowledgeCitation(t *testing.T) {
	answer := "Windows EDR 由内核采集、用户态代理、检测关联、响应执行和管理闭环组成。"
	citations := []retrieval.Citation{{KnowledgeID: "knowledge-1", ValidationState: "verified"}}
	if gaps := EDRAcceptanceGaps(answer, citations); len(gaps) != 0 {
		t.Fatalf("gaps=%v", gaps)
	}
	if gaps := EDRAcceptanceGaps("只讨论内核采集", nil); len(gaps) == 0 {
		t.Fatal("incomplete answer accepted")
	}
}
