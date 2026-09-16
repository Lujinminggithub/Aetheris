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
	allowed := GeneratedAnswer{Answer: "限速由信用桶机制控制。", Mode: AnalysisMode, Confidence: "medium", Details: "信用按输入速率消耗。", CitationNumbers: []int{1}}
	if err := ValidateGeneratedAnswer(allowed, []Citation{{Number: 1}}); err != nil {
		t.Fatal(err)
	}
	blocked := allowed
	blocked.Details = "查询来自 embedding 和 Qdrant。"
	if err := ValidateGeneratedAnswer(blocked, []Citation{{Number: 1}}); err == nil {
		t.Fatal("platform internals accepted")
	}
}
