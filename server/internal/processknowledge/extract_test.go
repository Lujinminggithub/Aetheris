package processknowledge

import (
	"strings"
	"testing"
	"time"
)

func TestExtractKnowledgeRequiresAIFinalAnswer(t *testing.T) {
	session := SessionDraft{ID: "session", LogicalProjectID: "safe", Turns: []TurnDraft{{
		ID: "question", Kind: HumanQuestion, Source: SourceTurn{EventID: "event-question", Content: "如何实现 EDR", OccurredAt: time.Now()},
	}}}
	if units := ExtractKnowledge(session); len(units) != 0 {
		t.Fatalf("user question became knowledge conclusion: %+v", units)
	}
}

func TestExtractKnowledgeUsesFinalAnswerAndConfirmedVerification(t *testing.T) {
	session := SessionDraft{ID: "session", LogicalProjectID: "safe", Turns: []TurnDraft{
		{ID: "question", Kind: HumanQuestion, Source: SourceTurn{EventID: "event-question", Content: "如何实现 EDR", OccurredAt: time.Now()}},
		{ID: "explore", Kind: AIExploration, Source: SourceTurn{EventID: "event-explore", Content: "我先分析", OccurredAt: time.Now()}},
		{ID: "answer", Kind: AIFinalAnswer, Source: SourceTurn{EventID: "event-answer", Content: "内核采集必须轻量，复杂检测放在用户态。" + longText(500), OccurredAt: time.Now()}},
		{ID: "confirm", Kind: HumanConfirmation, Source: SourceTurn{EventID: "event-confirm", Content: "确认", OccurredAt: time.Now()}},
		{ID: "test", Kind: TestResult, Source: SourceTurn{EventID: "event-test", Content: "全部测试通过", OccurredAt: time.Now()}},
	}}
	units := ExtractKnowledge(session)
	if len(units) != 1 {
		t.Fatalf("units=%d", len(units))
	}
	unit := units[0]
	if !strings.Contains(unit.Conclusion, "内核采集") || unit.DecisionState != "accepted" || unit.ValidationState != "verified" {
		t.Fatalf("unexpected unit: %+v", unit)
	}
	if len(unit.Evidence) < 4 {
		t.Fatalf("evidence=%+v", unit.Evidence)
	}
}

func TestRejectedAnswerIsFlushedBeforeLaterAssistantConclusion(t *testing.T) {
	session := SessionDraft{ID: "session", LogicalProjectID: "safe", Turns: []TurnDraft{
		{ID: "q", Kind: HumanQuestion, Source: SourceTurn{EventID: "q", Content: "分析服务通信问题"}},
		{ID: "a1", Kind: AIFinalAnswer, Source: SourceTurn{EventID: "a1", Content: "首次通信结论" + longText(500)}},
		{ID: "r", Kind: HumanRejection, Source: SourceTurn{EventID: "r", Content: "不对，该结论不成立"}},
		{ID: "a2", Kind: AIFinalAnswer, Source: SourceTurn{EventID: "a2", Content: "后续另一个任务结论" + longText(500)}},
	}}
	units := ExtractKnowledge(session)
	if len(units) != 1 || !strings.Contains(units[0].Conclusion, "首次通信结论") || units[0].DecisionState != "rejected" {
		t.Fatalf("units=%+v", units)
	}
}

func TestTopicComesFromHumanProblemNotIncidentalAnswerPath(t *testing.T) {
	if got := inferTopic("分析服务通信问题"); got != "研发过程知识" {
		t.Fatalf("topic=%s", got)
	}
	if got := inferTopic("DLP 的 OCR 方案"); got != "DLP" {
		t.Fatalf("topic=%s", got)
	}
	if got := inferTopic("Windows 如何实现 EDR"); got != "Windows EDR" {
		t.Fatalf("topic=%s", got)
	}
}

func TestConstraintAfterAnswerStartsNewKnowledgeAndDoesNotVerifyOldConclusion(t *testing.T) {
	session := SessionDraft{ID: "session", LogicalProjectID: "safe", Turns: []TurnDraft{
		{ID: "q1", Kind: HumanQuestion, Source: SourceTurn{EventID: "q1", Content: "分析通信故障"}},
		{ID: "a1", Kind: AIFinalAnswer, Source: SourceTurn{EventID: "a1", Content: "首次诊断结论" + longText(500)}},
		{ID: "c2", Kind: HumanConstraint, Source: SourceTurn{EventID: "c2", Content: "通信不能依赖 Broker"}},
		{ID: "a2", Kind: AIFinalAnswer, Source: SourceTurn{EventID: "a2", Content: "直连兼容层方案" + longText(500)}},
		{ID: "t2", Kind: TestResult, Source: SourceTurn{EventID: "t2", Content: "测试通过"}},
	}}
	units := ExtractKnowledge(session)
	if len(units) != 2 {
		t.Fatalf("units=%+v", units)
	}
	if units[0].ValidationState != "unverified" || !strings.Contains(units[0].Conclusion, "首次诊断") {
		t.Fatalf("first=%+v", units[0])
	}
	if units[1].ValidationState != "verified" || !strings.Contains(units[1].Conclusion, "直连兼容层") {
		t.Fatalf("second=%+v", units[1])
	}
}
