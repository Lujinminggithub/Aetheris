package retrieval

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestModelGatewayGeneratorParsesStructuredAnswer(t *testing.T) {
	var requestMode any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(request.Body).Decode(&body)
		contextValue, _ := body["context"].(map[string]any)
		requestMode = contextValue["answer_mode"]
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{"status": "ok", "result": `{"answer":"线路存在流量限制。","answer_mode":"direct","confidence":"high","details":"","citation_numbers":[1]}`})
	}))
	defer server.Close()

	answer, err := NewGenerator(server.URL, "token", 5*time.Second).GenerateAnswer(context.Background(), "线路有没有流量限制", DirectMode, []Citation{{Number: 1}}, "")

	if err != nil || answer.Answer != "线路存在流量限制。" || answer.Mode != DirectMode {
		t.Fatalf("answer=%#v err=%v", answer, err)
	}
	if requestMode != string(DirectMode) {
		t.Fatalf("mode=%v", requestMode)
	}
}

func TestModelGatewayGeneratorPromptKeepsTechnicalDetailsOutOfDirectMode(t *testing.T) {
	var prompt string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(request.Body).Decode(&body)
		messages, _ := body["messages"].([]any)
		if len(messages) > 0 {
			prompt, _ = messages[0].(map[string]any)["content"].(string)
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{"status": "ok", "result": `{"answer":"是。","answer_mode":"direct","confidence":"medium","details":"","citation_numbers":[]}`})
	}))
	defer server.Close()

	_, err := NewGenerator(server.URL, "token", 5*time.Second).GenerateAnswer(context.Background(), "线路有没有流量限制", DirectMode, nil, "")

	if err != nil || !strings.Contains(prompt, "不输出模型") || !strings.Contains(prompt, "80") {
		t.Fatalf("prompt=%q err=%v", prompt, err)
	}
}
