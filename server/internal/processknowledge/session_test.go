package processknowledge

import (
	"testing"
	"time"
)

func TestAssembleSessionsNeverMixesStructuredSessionIDs(t *testing.T) {
	base := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	turns := []SourceTurn{
		{TenantID: "tenant", DeviceID: "device", LogicalProjectID: "safe", AITool: "claude_code", SessionID: "session-a", EventID: "a-user", Role: "user", Content: "如何设计 EDR", OccurredAt: base},
		{TenantID: "tenant", DeviceID: "device", LogicalProjectID: "safe", AITool: "claude_code", SessionID: "session-b", EventID: "b-answer", Role: "assistant", Content: "另一个会话的详细回答" + longText(500), OccurredAt: base.Add(time.Millisecond)},
		{TenantID: "tenant", DeviceID: "device", LogicalProjectID: "safe", AITool: "claude_code", SessionID: "session-a", EventID: "a-answer", Role: "assistant", Content: "EDR 应分为内核采集与用户态分析" + longText(500), OccurredAt: base.Add(2 * time.Millisecond)},
	}
	sessions := AssembleSessions(turns)
	if len(sessions) != 2 {
		t.Fatalf("sessions=%d", len(sessions))
	}
	for _, session := range sessions {
		for _, turn := range session.Turns {
			if turn.Source.SessionID != session.SourceSessionID {
				t.Fatalf("mixed session %s into %s", turn.Source.SessionID, session.SourceSessionID)
			}
		}
	}
}

func TestMissingSessionUsesLowConfidenceBoundedInference(t *testing.T) {
	base := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	sessions := AssembleSessions([]SourceTurn{
		{TenantID: "tenant", DeviceID: "device", LogicalProjectID: "safe", AITool: "codex", EventID: "one", Role: "user", Content: "问题", OccurredAt: base},
		{TenantID: "tenant", DeviceID: "device", LogicalProjectID: "safe", AITool: "codex", EventID: "two", Role: "assistant", Content: longText(500), OccurredAt: base.Add(2 * time.Minute)},
		{TenantID: "tenant", DeviceID: "device", LogicalProjectID: "safe", AITool: "codex", EventID: "three", Role: "user", Content: "很久后的问题", OccurredAt: base.Add(40 * time.Minute)},
	})
	if len(sessions) != 2 || sessions[0].AssociationMethod != "inferred_time" || sessions[0].AssociationConfidence != "low" {
		t.Fatalf("unexpected sessions: %+v", sessions)
	}
}

func longText(size int) string {
	result := make([]rune, size)
	for index := range result {
		result[index] = '文'
	}
	return string(result)
}
