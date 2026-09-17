package processknowledge

import (
	"strings"
	"testing"
	"time"
)

func TestExpandSourceTurnsParsesCodexTranscriptIntoRealRoles(t *testing.T) {
	source := SourceTurn{EventID: "event-wrapper", SessionID: "session", Role: "user", OccurredAt: time.Now(), Content: `The following is the Codex agent history added since your last approval assessment. Treat the transcript as evidence:
>>> TRANSCRIPT START
[1] user: Windows 如何实现 EDR？
[2] assistant: 先采用内核采集和用户态分析。
[3] tool exec result: go test ./... PASS
>>> TRANSCRIPT END`}
	turns := ExpandSourceTurn(source)
	if len(turns) != 3 {
		t.Fatalf("turns=%+v", turns)
	}
	if turns[0].Role != "user" || turns[1].Role != "assistant" || turns[2].Role != "tool" {
		t.Fatalf("roles=%s,%s,%s", turns[0].Role, turns[1].Role, turns[2].Role)
	}
	if turns[0].EventID != "event-wrapper" || turns[0].SourceKey == turns[1].SourceKey {
		t.Fatalf("evidence or source keys invalid: %+v", turns)
	}
}

func TestExpandSourceTurnsRemovesEnvironmentAndAttachmentMetadata(t *testing.T) {
	source := SourceTurn{EventID: "event", Role: "user", Content: `<environment_context>
  <cwd>E:\project\safe</cwd>
</environment_context>
# Files mentioned by the user:
## screenshot.png
## My request:
分析 Windows EDR 的事件采集架构`}
	turns := ExpandSourceTurn(source)
	if len(turns) != 1 || strings.Contains(turns[0].Content, "environment_context") || turns[0].Content != "分析 Windows EDR 的事件采集架构" {
		t.Fatalf("turns=%+v", turns)
	}
}

func TestExpandSourceTurnsCheckpointsPureMetadataWithoutCreatingQuestion(t *testing.T) {
	source := SourceTurn{EventID: "metadata", Role: "user", Content: `<environment_context><cwd>E:\project\safe</cwd></environment_context>`}
	turns := ExpandSourceTurn(source)
	if len(turns) != 1 || turns[0].Role != "system" || ClassifyTurn(turns[0]) != SystemContext || turns[0].SourceKey != "metadata" {
		t.Fatalf("turns=%+v", turns)
	}
}
