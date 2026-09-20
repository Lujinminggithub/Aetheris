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

func TestQdrantPublicPointPayloadContainsNoPrivateScope(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/collections/public/points" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewQdrantClient(server.URL, "public", "", time.Second)
	err := client.Upsert(context.Background(), []Point{{
		ID: "point-1", DocumentID: "chunk-1", Public: true, PublicKnowledgeID: "public-1",
		CanonicalTopic: "Windows EDR", KnowledgeType: "implementation_pattern",
		ValidationState: "platform_certified", Revision: 2, ActivityType: "public_knowledge", Vector: []float32{0.1},
	}})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(requestBody)
	for _, forbidden := range []string{"tenant_id", "project_id", "device_id", "occurred_at", "event_id"} {
		if stringsContains(string(encoded), forbidden) {
			t.Fatalf("public payload leaks %s: %s", forbidden, encoded)
		}
	}
	for _, required := range []string{"public_knowledge_id", "public-1", "canonical_topic", "platform_certified"} {
		if !stringsContains(string(encoded), required) {
			t.Fatalf("public payload missing %s: %s", required, encoded)
		}
	}
}

func TestQdrantPublicQueryOmitsTenantFilter(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":{"points":[]},"status":"ok"}`))
	}))
	defer server.Close()

	client := NewQdrantClient(server.URL, "public", "", time.Second)
	if _, err := client.Query(context.Background(), []float32{0.1}, QueryFilter{Public: true, ActivityType: "public_knowledge"}, 10); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(requestBody["filter"])
	if stringsContains(string(encoded), "tenant_id") {
		t.Fatalf("public query contains tenant filter: %s", encoded)
	}
}

func TestQdrantDeleteUsesPointDeleteEndpoint(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/collections/public/points/delete" || r.URL.Query().Get("wait") != "true" {
			t.Fatalf("url = %s", r.URL.String())
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewQdrantClient(server.URL, "public", "", time.Second)
	if err := client.Delete(context.Background(), []string{"point-1", "point-2"}); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(requestBody)
	if !stringsContains(string(encoded), "point-1") || !stringsContains(string(encoded), "point-2") {
		t.Fatalf("delete body=%s", encoded)
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
