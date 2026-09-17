package retrieval

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestQdrantQueryAlwaysIncludesTenantAndScopeFilters(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/collections/activities/points/query" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":{"points":[]},"status":"ok"}`))
	}))
	defer server.Close()

	client := NewQdrantClient(server.URL, "activities", "", time.Second)
	_, err := client.Query(context.Background(), []float32{0.1, 0.2}, QueryFilter{TenantID: "tenant-1", DeviceID: "device-1", ProjectID: "project-1", ActivityType: "ai", KnowledgeVersion: 7}, 12)

	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(requestBody["filter"])
	for _, expected := range []string{"tenant-1", "device-1", "project-1", "ai", "knowledge_version", "7"} {
		if !stringsContains(string(encoded), expected) {
			t.Fatalf("filter %s missing %q", encoded, expected)
		}
	}
}

func stringsContains(value, target string) bool {
	for index := 0; index+len(target) <= len(value); index++ {
		if value[index:index+len(target)] == target {
			return true
		}
	}
	return false
}
