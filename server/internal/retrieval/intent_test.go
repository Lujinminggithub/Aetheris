package retrieval

import "testing"

func TestClassifyAnswerModeUsesQuestionGranularity(t *testing.T) {
	cases := map[string]AnswerMode{
		"线路有没有流量限制":   DirectMode,
		"线路是否存在限制":    DirectMode,
		"一共有多少次失败":    NumericMode,
		"为什么线路会限速":    ReasonMode,
		"怎么配置线路限速":    ProcedureMode,
		"详细分析限速实现和证据": AnalysisMode,
		"线路情况":        DirectMode,
	}
	for question, want := range cases {
		if got := ClassifyAnswerMode(question); got != want {
			t.Fatalf("%q: got %q want %q", question, got, want)
		}
	}
}

func TestAnswerModeValidationRejectsUnknownValue(t *testing.T) {
	if !DirectMode.Valid() || AnswerMode("verbose").Valid() {
		t.Fatal("answer mode validation mismatch")
	}
}
