package retrieval

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestKnowledgeAnswerBuildsDeterministicPlanBeforeGenerating(t *testing.T) {
	tasks := []string{}
	prompts := []string{}
	var answerPlan map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(request.Body).Decode(&body)
		task, _ := body["task"].(string)
		tasks = append(tasks, task)
		messages, _ := body["messages"].([]any)
		if len(messages) > 0 {
			prompt, _ := messages[0].(map[string]any)["content"].(string)
			prompts = append(prompts, prompt)
		}
		contextValue, _ := body["context"].(map[string]any)
		answerPlan, _ = contextValue["answer_plan"].(map[string]any)
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{"status": "ok", "result": `{"answer":"Windows EDR 采用内核与用户态协作。","answer_mode":"analysis","confidence":"high","details":"内核热路径保持轻量。","citation_numbers":[1]}`})
	}))
	defer server.Close()
	citations := []Citation{{Number: 1, KnowledgeID: "knowledge-1", SourceKind: "process_knowledge", ValidationState: "verified", Applicability: "Windows 高 IRQL 路径", Excerpt: "内核采集保持轻量"}}
	answer, err := NewGenerator(server.URL, "token", 5*time.Second).GenerateAnswer(context.Background(), "详细分析 Windows EDR", AnalysisMode, citations, "")
	if err != nil || answer.Confidence != "high" || len(tasks) != 1 || tasks[0] != "rag_answer" {
		t.Fatalf("tasks=%v answer=%+v err=%v", tasks, answer, err)
	}
	if len(prompts) != 1 || !strings.Contains(prompts[0], "220") {
		t.Fatalf("prompts=%v", prompts)
	}
	if answerPlan == nil || answerPlan["question_intent"] != "详细分析 Windows EDR" {
		t.Fatalf("answer plan=%v", answerPlan)
	}
	if !strings.Contains(fmt.Sprint(answerPlan["required_topics"]), "内核采集") || !strings.Contains(fmt.Sprint(answerPlan["required_topics"]), "管理闭环") {
		t.Fatalf("EDR required topics=%v", answerPlan["required_topics"])
	}
	claims, _ := answerPlan["claims"].([]any)
	if len(claims) != 1 || !strings.Contains(fmt.Sprint(claims[0]), "knowledge-1") {
		t.Fatalf("claims=%v", claims)
	}
}

func TestDLPAnswerPlanRequiresCollectionDetectionAndResponseTopics(t *testing.T) {
	plan := buildAnswerPlan("如何在 Windows 实现 DLP", AnalysisMode, []Citation{{KnowledgeID: "knowledge-1", Topic: "DLP", Excerpt: "结论：图片经 OCR 后进入规则引擎并执行阻断。"}})
	joined := strings.Join(plan.RequiredTopics, " ")
	for _, topic := range []string{"内容采集", "OCR与文本提取", "规则检测", "阻断与审计"} {
		if !strings.Contains(joined, topic) {
			t.Fatalf("plan topics=%v", plan.RequiredTopics)
		}
	}
	if len(plan.Claims) != 1 || !strings.Contains(plan.Claims[0].Claim, "OCR") || !strings.Contains(plan.Claims[0].Claim, "阻断") {
		t.Fatalf("plan claims=%v", plan.Claims)
	}
}

func TestModelEvidenceRemovesProcessNarrationAndKeepsConclusion(t *testing.T) {
	excerpt := "主题：DLP\n结论：这看起来是一个架构设计任务。我先给出方案，确认后再拆分实现任务。\n\n当前仓库已经具备 OCR 基础能力，图片识别后进入规则引擎并执行阻断。\n依据：event-1"
	content := modelEvidenceContent(Citation{Excerpt: excerpt})
	if strings.Contains(content, "架构设计任务") || strings.Contains(content, "确认后") || !strings.Contains(content, "OCR") || !strings.Contains(content, "规则引擎") {
		t.Fatalf("content=%q", content)
	}
}

func TestKnowledgeAnswerBoundsEvidenceForLocalModel(t *testing.T) {
	evidenceCounts := []int{}
	contentLengths := []int{}
	verboseMetadata := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(request.Body).Decode(&body)
		contextValue, _ := body["context"].(map[string]any)
		evidence, _ := contextValue["evidence"].([]any)
		evidenceCounts = append(evidenceCounts, len(evidence))
		for _, raw := range evidence {
			item, _ := raw.(map[string]any)
			if _, ok := item["fact_id"]; ok {
				verboseMetadata = true
			}
			if _, ok := item["occurred_at"]; ok {
				verboseMetadata = true
			}
			content, _ := item["content"].(string)
			contentLengths = append(contentLengths, len([]rune(content)))
		}
		writer.Header().Set("Content-Type", "application/json")
		if body["task"] == "plan_rag_answer" {
			_ = json.NewEncoder(writer).Encode(map[string]any{"status": "ok", "result": `{"question_intent":"分析 EDR","required_topics":["架构"],"claims":[{"claim":"采用分层架构","source_kind":"process_knowledge","knowledge_unit_ids":["knowledge-1"],"applicability":"Windows"}],"coverage_gaps":[]}`})
			return
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"status": "ok", "result": `{"answer":"采用分层架构。","answer_mode":"analysis","confidence":"medium","details":"结合本地过程知识。","citation_numbers":[1]}`})
	}))
	defer server.Close()
	citations := make([]Citation, 8)
	for index := range citations {
		citations[index] = Citation{Number: index + 1, KnowledgeID: fmt.Sprintf("knowledge-%d", index+1), Excerpt: strings.Repeat("中", 1000)}
	}
	_, err := NewGenerator(server.URL, "token", 5*time.Second).GenerateAnswer(context.Background(), "详细分析 Windows EDR", AnalysisMode, citations, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(evidenceCounts) != 1 || evidenceCounts[0] != 3 {
		t.Fatalf("evidence counts=%v", evidenceCounts)
	}
	if verboseMetadata {
		t.Fatal("模型证据不应重复携带展示元数据")
	}
	for _, length := range contentLengths {
		if length > 500 {
			t.Fatalf("evidence content length=%d", length)
		}
	}
}
