package retrieval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type QdrantClient struct {
	url, collection, apiKey string
	client                  *http.Client
	mu                      sync.Mutex
	readyDimensions         int
}

func NewQdrantClient(baseURL, collection, apiKey string, timeout time.Duration) *QdrantClient {
	return &QdrantClient{url: strings.TrimRight(baseURL, "/"), collection: collection, apiKey: apiKey, client: &http.Client{Timeout: timeout}}
}

func (client *QdrantClient) EnsureCollection(ctx context.Context, dimensions int) error {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.readyDimensions == dimensions {
		return nil
	}
	path := "/collections/" + url.PathEscape(client.collection)
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, client.url+path, nil)
	response, err := client.do(request)
	if err == nil && response.StatusCode == http.StatusOK {
		response.Body.Close()
		if err := client.ensurePayloadIndexes(ctx); err != nil {
			return err
		}
		client.readyDimensions = dimensions
		return nil
	}
	if response != nil {
		response.Body.Close()
	}
	body := map[string]any{"vectors": map[string]any{"size": dimensions, "distance": "Cosine"}, "on_disk_payload": true}
	response, err = client.request(ctx, http.MethodPut, path, body)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("qdrant_collection_status_%d", response.StatusCode)
	}
	if err := client.ensurePayloadIndexes(ctx); err != nil {
		return err
	}
	client.readyDimensions = dimensions
	return nil
}

func (client *QdrantClient) ensurePayloadIndexes(ctx context.Context) error {
	for field, schema := range map[string]string{"tenant_id": "keyword", "device_id": "keyword", "project_id": "keyword", "activity_type": "keyword", "occurred_at": "datetime", "knowledge_version": "integer"} {
		response, err := client.request(ctx, http.MethodPut, "/collections/"+url.PathEscape(client.collection)+"/index?wait=true", map[string]any{"field_name": field, "field_schema": schema})
		if err != nil {
			return err
		}
		response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return fmt.Errorf("qdrant_index_status_%d", response.StatusCode)
		}
	}
	return nil
}

func (client *QdrantClient) Upsert(ctx context.Context, points []Point) error {
	values := make([]map[string]any, 0, len(points))
	for _, point := range points {
		payload := map[string]any{"document_id": point.DocumentID, "activity_type": point.ActivityType}
		if point.Public {
			payload["public_knowledge_id"] = point.PublicKnowledgeID
			payload["canonical_topic"] = point.CanonicalTopic
			payload["knowledge_type"] = point.KnowledgeType
			payload["validation_state"] = point.ValidationState
			payload["domains"] = point.Domains
			payload["entities"] = point.Entities
			payload["revision"] = point.Revision
		} else {
			payload["tenant_id"] = point.TenantID
			payload["device_id"] = point.DeviceID
			payload["project_id"] = point.ProjectID
			payload["occurred_at"] = point.OccurredAt.Format(time.RFC3339Nano)
			payload["knowledge_version"] = point.KnowledgeVersion
		}
		values = append(values, map[string]any{"id": point.ID, "vector": point.Vector, "payload": payload})
	}
	response, err := client.request(ctx, http.MethodPut, "/collections/"+url.PathEscape(client.collection)+"/points?wait=true", map[string]any{"points": values})
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("qdrant_upsert_status_%d", response.StatusCode)
	}
	return nil
}

func (client *QdrantClient) Delete(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	response, err := client.request(ctx, http.MethodPost, "/collections/"+url.PathEscape(client.collection)+"/points/delete?wait=true", map[string]any{"points": ids})
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("qdrant_delete_status_%d", response.StatusCode)
	}
	return nil
}

func (client *QdrantClient) Query(ctx context.Context, vector []float32, filter QueryFilter, limit int) ([]Hit, error) {
	must := []map[string]any{}
	if !filter.Public {
		must = append(must, map[string]any{"key": "tenant_id", "match": map[string]any{"value": filter.TenantID}})
	}
	for key, value := range map[string]string{"device_id": filter.DeviceID, "project_id": filter.ProjectID, "activity_type": filter.ActivityType} {
		if value != "" {
			must = append(must, map[string]any{"key": key, "match": map[string]any{"value": value}})
		}
	}
	if filter.KnowledgeVersion > 0 {
		must = append(must, map[string]any{"key": "knowledge_version", "match": map[string]any{"value": filter.KnowledgeVersion}})
	}
	if !filter.From.IsZero() || !filter.ToExclusive.IsZero() {
		bounds := map[string]any{}
		if !filter.From.IsZero() {
			bounds["gte"] = filter.From.Format(time.RFC3339)
		}
		if !filter.ToExclusive.IsZero() {
			bounds["lt"] = filter.ToExclusive.Format(time.RFC3339)
		}
		must = append(must, map[string]any{"key": "occurred_at", "range": bounds})
	}
	response, err := client.request(ctx, http.MethodPost, "/collections/"+url.PathEscape(client.collection)+"/points/query", map[string]any{"query": vector, "filter": map[string]any{"must": must}, "limit": limit, "with_payload": true, "with_vector": false})
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("qdrant_query_status_%d", response.StatusCode)
	}
	var result struct {
		Result struct {
			Points []struct {
				Score   float64
				Payload map[string]any
			} `json:"points"`
		} `json:"result"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return nil, err
	}
	hits := make([]Hit, 0, len(result.Result.Points))
	for _, point := range result.Result.Points {
		if id, ok := point.Payload["document_id"].(string); ok && id != "" {
			hits = append(hits, Hit{id, point.Score})
		}
	}
	return hits, nil
}

func (client *QdrantClient) Health(ctx context.Context) error {
	response, err := client.request(ctx, http.MethodGet, "/readyz", nil)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("qdrant_unavailable")
	}
	return nil
}
func (client *QdrantClient) request(ctx context.Context, method, path string, value any) (*http.Response, error) {
	var body *bytes.Reader
	if value == nil {
		body = bytes.NewReader(nil)
	} else {
		raw, _ := json.Marshal(value)
		body = bytes.NewReader(raw)
	}
	request, err := http.NewRequestWithContext(ctx, method, client.url+path, body)
	if err != nil {
		return nil, err
	}
	if value != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	return client.do(request)
}
func (client *QdrantClient) do(request *http.Request) (*http.Response, error) {
	if client.apiKey != "" {
		request.Header.Set("api-key", client.apiKey)
	}
	return client.client.Do(request)
}
