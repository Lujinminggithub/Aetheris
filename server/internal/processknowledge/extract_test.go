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
