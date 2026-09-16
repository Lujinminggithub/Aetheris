package episodes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOllamaSummaryRejectsEvidenceOutsideInput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"response": `{"title":"修复登录","objective":"修复登录","outcome":"已完成","evidence":[{"event_id":"event-unknown","section":"outcome"}]}`})
	}))
	defer server.Close()

	_, err := SummarizeWithOllama(context.Background(), server.Client(), server.URL, "qwen", Episode{EpisodeID: "episode-1", Objective: "修复登录", Evidence: []Evidence{{EventID: "event-1", Section: "objective"}}})
	if err == nil || !strings.Contains(err.Error(), "证据") {
		t.Fatalf("expected evidence validation error, got %v", err)
	}
}

func TestOllamaSummaryReturnsValidatedChineseProjection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request["model"] != "qwen" {
			t.Fatalf("model=%v", request["model"])
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"response": `{"title":"修复登录","objective":"修复登录","outcome":"验证通过","evidence":[{"event_id":"event-1","section":"objective"}]}`})
	}))
	defer server.Close()

	result, err := SummarizeWithOllama(context.Background(), server.Client(), server.URL, "qwen", Episode{EpisodeID: "episode-1", Objective: "修复登录", Evidence: []Evidence{{EventID: "event-1", Section: "objective"}}})
	if err != nil || result.Title != "修复登录" || len(result.Evidence) != 1 {
		t.Fatalf("unexpected summary=%+v err=%v", result, err)
	}
}
