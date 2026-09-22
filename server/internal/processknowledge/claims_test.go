package processknowledge

import "testing"

func TestAtomizeKnowledgeCreatesScopedClaimsAndPreservesEvidence(t *testing.T) {
	claims := AtomizeKnowledge(KnowledgeDraft{ID: "knowledge", Topic: "DLP", Problem: "Windows DLP 如何实现内容阻断", Conclusion: "采集文件和剪贴板内容，图片通过 OCR 提取文本。\n- 命中策略后阻断并记录审计。", Applicability: "Windows 11", ValidationState: "verified"}, 1, []string{"evidence-1"})
	if len(claims) != 2 {
		t.Fatalf("claims=%+v", claims)
	}
	for _, claim := range claims {
		if claim.Domain != "dlp" || len(claim.Entities) == 0 || len(claim.EvidenceIDs) != 1 || claim.ValidationState != "verified" {
			t.Fatalf("claim=%+v", claim)
		}
	}
}

func TestAtomizeKnowledgeRejectsWorkflowAndCrossDomainContent(t *testing.T) {
	if claims := AtomizeKnowledge(KnowledgeDraft{ID: "agent", Topic: "研发过程知识", Problem: "继续规格实施", Conclusion: "Agent message from /root: FINAL_ANSWER src/nb_live.c:33 FEC 修复"}, 1, nil); len(claims) != 0 {
		t.Fatalf("workflow claim=%+v", claims)
	}
	if claims := AtomizeKnowledge(KnowledgeDraft{ID: "cross", Topic: "DLP", Problem: "Windows DLP 如何阻断", Conclusion: "FEC 恢复公网丢包并调整媒体队列，避免首包过期"}, 1, nil); len(claims) != 0 {
		t.Fatalf("cross-domain claim=%+v", claims)
	}
}

func TestAtomizeKnowledgeUsesProblemDomainInsteadOfPollutedTopic(t *testing.T) {
	claims := AtomizeKnowledge(KnowledgeDraft{ID: "mislabeled", Topic: "DLP", Problem: "UDP 下行丢包如何处理", Conclusion: "FEC 增加冗余分片以恢复公网丢包。", ValidationState: "unverified"}, 1, nil)
	if len(claims) != 1 || claims[0].Domain != "network_transport" {
		t.Fatalf("claims=%+v", claims)
	}
}
