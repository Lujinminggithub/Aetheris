package processknowledge

import (
	"strings"
	"testing"
)

func TestChunkKnowledgeIndexesLateContentAndKeepsAPIIdentifier(t *testing.T) {
	late := "Windows EDR 通过 PsSetCreateProcessNotifyRoutineEx 和 WFP 采集事件。"
	unit := KnowledgeDraft{
		ID: "knowledge", Topic: "Windows EDR", Problem: "如何实现 EDR",
		Conclusion: strings.Repeat("前置分析。", 7000) + late + strings.Repeat("后续边界。", 1000),
	}
	chunks := ChunkKnowledge(unit, 600, 1000)
	if len(chunks) < 2 {
		t.Fatalf("chunks=%d", len(chunks))
	}
	found := false
	for _, chunk := range chunks {
		if strings.Contains(chunk.Content, "PsSetCreateProcessNotifyRoutineEx") {
			found = true
		}
		if len([]rune(chunk.Content)) > 1100 {
			t.Fatalf("oversized chunk: %d", len([]rune(chunk.Content)))
		}
	}
	if !found {
		t.Fatal("late EDR content was not indexed")
	}
}
