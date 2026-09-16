package retrieval

import "strings"

type AnswerMode string

const (
	DirectMode    AnswerMode = "direct"
	NumericMode   AnswerMode = "numeric"
	ReasonMode    AnswerMode = "reason"
	ProcedureMode AnswerMode = "procedure"
	AnalysisMode  AnswerMode = "analysis"
)

func (mode AnswerMode) Valid() bool {
	switch mode {
	case DirectMode, NumericMode, ReasonMode, ProcedureMode, AnalysisMode:
		return true
	default:
		return false
	}
}

func ClassifyAnswerMode(question string) AnswerMode {
	value := strings.ToLower(strings.Join(strings.Fields(question), " "))
	if containsAny(value, "详细", "深入", "完整分析", "实现", "机制", "算法", "证据", "对比", "in detail", "implementation", "evidence", "compare") {
		return AnalysisMode
	}
	if containsAny(value, "怎么", "如何", "步骤", "怎样", "操作方法", "how to", "steps") {
		return ProcedureMode
	}
	if containsAny(value, "为什么", "为何", "原因", "根因", "why", "reason") {
		return ReasonMode
	}
	if containsAny(value, "多少", "几次", "多久", "比例", "数量", "多长", "how many", "how much", "percentage") {
		return NumericMode
	}
	return DirectMode
}

func containsAny(value string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(value, term) {
			return true
		}
	}
	return false
}
