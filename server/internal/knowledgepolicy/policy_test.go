package knowledgepolicy

import "testing"

func TestStrictDomainCompatibilityRejectsDLPNetworkDrift(t *testing.T) {
	question := Domains("Windows DLP 的实现原理")
	if !Contains(question, DomainDLP) {
		t.Fatalf("question domains=%v", question)
	}
	if DomainsCompatible(question, Domains("FEC 恢复公网 UDP 丢包并调整媒体队列")) {
		t.Fatal("network transport accepted for DLP")
	}
	if !DomainsCompatible(question, Domains("DLP 采集剪贴板并通过 OCR 匹配策略后阻断")) {
		t.Fatal("DLP evidence rejected")
	}
}

func TestOrchestrationMarkersAreNotKnowledge(t *testing.T) {
	if !IsOrchestration("委派子代理继续规格实施", "Agent message from /root/reviewer: FINAL_ANSWER") {
		t.Fatal("orchestration markers were not detected")
	}
}
