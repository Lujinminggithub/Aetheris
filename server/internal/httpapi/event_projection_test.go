package httpapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/aetheris-dev/aetheris/server/internal/events"
)

func TestAdminEventProjectionRemovesLocalProjectPaths(t *testing.T) {
	event := events.Event{EventType: "ai.message", Payload: map[string]any{"project": `E:\code\jtagent`, "content": "安全内容"}, Provenance: map[string]any{"project_root": `E:\code\jtagent`, "source_file": "session.jsonl"}}

	projected := projectEventForAdmin(event)
	raw, _ := json.Marshal(projected)

	if strings.Contains(string(raw), `E:\\code`) || projected.Payload["project_label"] != "jtagent" || projected.Payload["content"] != "安全内容" {
		t.Fatalf("projection = %s", raw)
	}
}
