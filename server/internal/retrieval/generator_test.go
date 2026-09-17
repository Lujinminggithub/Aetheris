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

func TestKnowledgeAnswerPlansClaimsBeforeGenerating(t *testing.T) {
	tasks := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(request.Body).Decode(&body)
		task, _ := body["task"].(string)
		tasks = append(tasks, task)
		writer.Header().Set("Content-Type", "application/json")
		if task == "plan_rag_answer" {
			_ = json.NewEncoder(writer).Encode(map[string]any{"status": "ok", "result": `{"question_intent":"分析 EDR","required_topics":["总体架构","采集","检测","响应"],"claims":[{"claim":"内核采集保持轻量","source_kind":"verified_process_knowledge","knowledge_unit_ids":["knowledge-1"],"applicability":"Windows 高 IRQL 路径"}],"coverage_gaps":[]}`})
			return
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"status": "ok", "result": `{"answer":"Windows EDR 采用内核与用户态协作。","answer_mode":"analysis","confidence":"high","details":"内核热路径保持轻量。","citation_numbers":[1]}`})
	}))
	defer server.Close()
	citations := []Citation{{Number: 1, KnowledgeID: "knowledge-1", SourceKind: "process_knowledge", ValidationState: "verified", Applicability: "Windows 高 IRQL 路径", Excerpt: "内核采集保持轻量"}}
	answer, err := NewGenerator(server.URL, "token", 5*time.Second).GenerateAnswer(context.Background(), "详细分析 Windows EDR", AnalysisMode, citations, "")
	if err != nil || answer.Confidence != "high" || len(tasks) != 2 || tasks[0] != "plan_rag_answer" || tasks[1] != "rag_answer" {
		t.Fatalf("tasks=%v answer=%+v err=%v", tasks, answer, err)
	}
}
