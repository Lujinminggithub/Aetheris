package publicknowledge

import (
	"errors"
	"testing"
)

func TestBuildCandidateRequiresVerifiedOrAcceptedPrivateKnowledge(t *testing.T) {
	_, err := BuildCandidate(PrivateKnowledge{
		SourceTenantID: "tenant-a", KnowledgeID: "knowledge-a", Revision: 1,
		Topic: "Windows EDR", KnowledgeType: "implementation_pattern",
		Problem: "如何实现 EDR", Conclusion: "采用内核采集与用户态分析",
		DecisionState: "proposed", ValidationState: "unverified",
	})
	if !errors.Is(err, ErrCandidateIneligible) {
		t.Fatalf("err=%v", err)
	}
}

func TestBuildCandidateCreatesSanitizedPendingReview(t *testing.T) {
	candidate, err := BuildCandidate(PrivateKnowledge{
		SourceTenantID: "tenant-a", KnowledgeID: "knowledge-a", Revision: 2, SessionID: "session-a",
		Topic: "Windows EDR", KnowledgeType: "implementation_pattern",
		Problem: "如何在 E:\\project\\safe 实现 EDR", Conclusion: "内核回调保持轻量，复杂分析放在用户态。",
		Rationale: "已运行驱动集成测试", Applicability: "Windows 11", DecisionState: "accepted", ValidationState: "verified",
		SourceContentHash: "source-hash-a", SensitiveTerms: []string{"safe"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Unit.PublicationState != PendingReview || candidate.Revision.ValidationState != EvidenceVerified {
		t.Fatalf("candidate=%+v", candidate)
	}
	if candidate.Revision.CanonicalHash == "" || candidate.Source.IndependenceGroup == "" {
		t.Fatalf("candidate identifiers are empty: %+v", candidate)
	}
	if candidate.Source.SourceTenantID != "tenant-a" || candidate.Unit.ID == "" {
		t.Fatalf("source linkage is incomplete: %+v", candidate)
	}
}

func TestIndependenceGroupDoesNotCountCopiedOrCommonExternalSourcesTwice(t *testing.T) {
	base := PrivateKnowledge{SourceTenantID: "tenant-a", SessionID: "session-a", SourceContentHash: "same-answer"}
	copy := base
	copy.SourceTenantID, copy.SessionID = "tenant-b", "session-b"
	if independenceGroup(base) != independenceGroup(copy) {
		t.Fatal("identical AI content must share one independence group")
	}
	base.ExternalSourceHash, copy.ExternalSourceHash = "same-url", "same-url"
	base.SourceContentHash, copy.SourceContentHash = "answer-a", "answer-b"
	if independenceGroup(base) != independenceGroup(copy) {
		t.Fatal("the same external source must share one independence group")
	}
}

func TestLikelyConflictRequiresSameScopeAndOppositePolarity(t *testing.T) {
	if !likelyConflict("高 IRQL 路径如何处理", "必须在内核回调中执行阻塞分析", "Windows 11", "高 IRQL 路径如何处理", "禁止在内核回调中执行阻塞分析", "Windows 11") {
		t.Fatal("opposite conclusions in the same scope were not detected")
	}
	if likelyConflict("如何实现 EDR", "采集进程事件", "Windows 11", "如何实现 EDR", "采集网络事件", "Windows 11") {
		t.Fatal("complementary conclusions were marked as conflict")
	}
	if likelyConflict("如何实现 EDR", "可以启用 ETW", "Windows 10", "如何实现 EDR", "不支持 ETW", "Windows 11") {
		t.Fatal("different applicability was marked as conflict")
	}
}
