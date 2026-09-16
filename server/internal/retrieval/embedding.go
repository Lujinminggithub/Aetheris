package retrieval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type ModelGatewayEmbeddingClient struct {
	url, token, model string
	client            *http.Client
}

func NewEmbeddingClient(url, token, model string, timeout time.Duration) *ModelGatewayEmbeddingClient {
	return &ModelGatewayEmbeddingClient{strings.TrimRight(url, "/"), token, model, &http.Client{Timeout: timeout}}
}

func (client *ModelGatewayEmbeddingClient) Embed(ctx context.Context, inputs []string) ([][]float32, error) {
	body, _ := json.Marshal(map[string]any{"model": client.model, "inputs": inputs})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.url+"/internal/v1/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+client.token)
	response, err := client.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	var result struct {
		Status, Error string
		Embeddings    [][]float32
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK || result.Status != "succeeded" || len(result.Embeddings) != len(inputs) {
		return nil, fmt.Errorf("embedding_failed:%s", result.Error)
	}
	return result.Embeddings, nil
}
