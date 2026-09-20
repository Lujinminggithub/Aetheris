package retrieval

import (
	"strings"
	"testing"
)

func TestDirectAnswerAcceptsConciseBusinessConclusion(t *testing.T) {
	answer := GeneratedAnswer{Answer: "线路存在流量限制。", Mode: DirectMode, Confidence: "high", CitationNumbers: []int{1}}
	if err := ValidateGeneratedAnswer(answer, []Citation{{Number: 1}}); err != nil {
		t.Fatal(err)
	}
}

func TestGeneratedAnswerRejectsInternalDetailsLengthAndFalseCitations(t *testing.T) {
	cases := []GeneratedAnswer{
		{Answer: "线路存在限制，Qdrant 向量索引尚未读取服务器日志。", Mode: DirectMode, Confidence: "high", CitationNumbers: []int{1}},
		{Answer: strings.Repeat("线路存在流量限制", 12), Mode: DirectMode, Confidence: "high", CitationNumbers: []int{1}},
		{Answer: "线路存在流量限制。", Mode: DirectMode, Confidence: "high", Details: "信用桶实现", CitationNumbers: []int{1}},
		{Answer: "线路存在流量限制。", Mode: DirectMode, Confidence: "high", CitationNumbers: []int{2}},
		{Answer: "线路存在流量限制。", Mode: DirectMode, Confidence: "high", CitationNumbers: []int{}},
		{Answer: "线路存在流量限制。", Mode: AnswerMode("verbose"), Confidence: "high", CitationNumbers: []int{1}},
	}
	for index, answer := range cases {
		if err := ValidateGeneratedAnswer(answer, []Citation{{Number: 1}}); err == nil {
			t.Fatalf("case %d accepted: %#v", index, answer)
		}
	}
}

func TestParseGeneratedAnswerRequiresStrictJSONObject(t *testing.T) {
	valid := `{"answer":"线路存在流量限制。","answer_mode":"direct","confidence":"high","details":"","citation_numbers":[1]}`
	answer, err := ParseGeneratedAnswer(valid)
	if err != nil || answer.Mode != DirectMode {
		t.Fatalf("answer=%#v err=%v", answer, err)
	}
	for _, invalid := range []string{
		"```json\n" + valid + "\n```",
		valid + " trailing",
		`{"answer":"是。","answer_mode":"direct","confidence":"high","details":"","citation_numbers":[1],"extra":true}`,
	} {
		if _, err := ParseGeneratedAnswer(invalid); err == nil {
			t.Fatalf("invalid response accepted: %q", invalid)
		}
	}
}

func TestAnalysisModeAllowsDomainImplementationDetailsButNotPlatformInternals(t *testing.T) {
	allowed := GeneratedAnswer{Answer: "限速由信用桶机制控制。", Mode: AnalysisMode, Confidence: "medium", Details: "系统按输入速率持续消耗信用额度，额度耗尽后进入稳定限速；恢复阶段按时间补充额度，并结合连接状态避免短时突发流量反复触发切换。", CitationNumbers: []int{1}}
	if err := ValidateGeneratedAnswer(allowed, []Citation{{Number: 1}}); err != nil {
		t.Fatal(err)
	}
	blocked := allowed
	blocked.Details = "查询来自 embedding 和 Qdrant。"
	if err := ValidateGeneratedAnswer(blocked, []Citation{{Number: 1}}); err == nil {
		t.Fatal("platform internals accepted")
	}
}

func TestAnalysisAnswerAllowsDomainOCRWorker(t *testing.T) {
	answer := GeneratedAnswer{Answer: "Windows DLP 使用分层内容检测链路。", Mode: AnalysisMode, Confidence: "low", Details: "客户端采集文件、剪贴板和截图；图片由 OCR Worker 池完成识别，文本规范化后进入规则引擎，再依据策略执行放行、记录、告警或阻断，并限制原文留存以保护隐私。", CitationNumbers: []int{1}}
	if err := ValidateGeneratedAnswer(answer, []Citation{{Number: 1, KnowledgeID: "knowledge-1", ValidationState: "unverified"}}); err != nil {
		t.Fatal(err)
	}
}

func TestHighConfidenceKnowledgeAnswerRequiresVerifiedEvidence(t *testing.T) {
	answer := GeneratedAnswer{Answer: "内核采集与用户态分析协作。", Mode: AnalysisMode, Confidence: "high", CitationNumbers: []int{1}}
	allowed := []Citation{{Number: 1, KnowledgeID: "knowledge-1", ValidationState: "unverified", SourceKind: "process_knowledge"}}
	if err := ValidateGeneratedAnswer(answer, allowed); err == nil {
		t.Fatal("high confidence accepted unverified process knowledge")
	}
}

func TestHighConfidenceAllowsPlatformCertifiedPublicKnowledge(t *testing.T) {
	answer := GeneratedAnswer{Answer: "内核采集与用户态分析协作。", Mode: AnalysisMode, Confidence: "high", Details: "内核路径保持轻量，只完成必要事件采集；复杂检测、关联和响应在用户态完成，并按适用条件控制系统版本与回调范围。", CitationNumbers: []int{1}}
	allowed := []Citation{{Number: 1, KnowledgeID: "public-1", PublicKnowledgeID: "public-1", SourceScope: "platform_public", ValidationState: "platform_certified"}}
	if err := ValidateGeneratedAnswer(answer, allowed); err != nil {
		t.Fatal(err)
	}
}

func TestUnverifiedKnowledgeAnswerCannotClaimLocalVerification(t *testing.T) {
	answer := GeneratedAnswer{Answer: "Windows DLP 使用 OCR 与规则检测。", Mode: AnalysisMode, Confidence: "medium", Details: "本地证据未验证，通用知识用于补全。已验证本地结果表明系统可以执行内容识别和阻断，因此可直接作为确定结论。", CitationNumbers: []int{1}}
	allowed := []Citation{{Number: 1, KnowledgeID: "knowledge-1", ValidationState: "unverified", SourceKind: "process_knowledge"}}
	if err := ValidateGeneratedAnswer(answer, allowed); err == nil {
		t.Fatal("unverified evidence was presented as verified")
	}
}

func TestAnalysisAnswerRejectsGenericShortOutput(t *testing.T) {
	answer := GeneratedAnswer{Answer: "分析", Mode: AnalysisMode, Confidence: "low", Details: "本地证据未验证，通用知识用于补全。", CitationNumbers: []int{1}}
	if err := ValidateGeneratedAnswer(answer, []Citation{{Number: 1, KnowledgeID: "knowledge-1", ValidationState: "unverified"}}); err == nil {
		t.Fatal("generic short analysis was accepted")
	}
}

func TestAnalysisAnswerRejectsProcessNarrationInsteadOfAnswer(t *testing.T) {
	answer := GeneratedAnswer{Answer: "Windows DLP 需要结合 OCR 能力。", Mode: AnalysisMode, Confidence: "low", Details: "这看起来是一个架构设计任务，建议先完成架构设计，后续确认后再拆分实现任务。当前证据尚未验证，因此暂时不提供具体采集、检测和阻断机制。", CitationNumbers: []int{1}}
	if err := ValidateGeneratedAnswer(answer, []Citation{{Number: 1, KnowledgeID: "knowledge-1", ValidationState: "unverified"}}); err == nil {
		t.Fatal("process narration was accepted as an answer")
	}
}
