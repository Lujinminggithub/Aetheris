package episodes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type OllamaSummary struct {
	Title     string     `json:"title"`
	Objective string     `json:"objective"`
	Outcome   string     `json:"outcome"`
	Evidence  []Evidence `json:"evidence"`
}

func SummarizeWithOllama(ctx context.Context, client *http.Client, endpoint, model string, episode Episode) (OllamaSummary, error) {
	if client == nil {
		client = http.DefaultClient
	}
	if strings.TrimSpace(endpoint) == "" || strings.TrimSpace(model) == "" {
		return OllamaSummary{}, fmt.Errorf("模型服务未配置")
	}
	input, _ := json.Marshal(map[string]any{
		"episode":  map[string]any{"objective": episode.Objective, "actions": episode.Actions, "validations": episode.Validations},
		"evidence": episode.Evidence,
	})
	prompt := "只根据以下 JSON 生成严格 JSON，字段必须为 title、objective、outcome、evidence；evidence 只能引用输入中存在的 event_id；不得添加解释文字。输入：" + string(input)
	body, _ := json.Marshal(map[string]any{"model": model, "prompt": prompt, "stream": false, "format": "json"})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(endpoint, "/")+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return OllamaSummary{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return OllamaSummary{}, err
	}
	defer response.Body.Close()
	if response.StatusCode >= 400 {
		return OllamaSummary{}, fmt.Errorf("模型服务返回 HTTP %d", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 256*1024))
	if err != nil {
		return OllamaSummary{}, err
	}
	var envelope struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || strings.TrimSpace(envelope.Response) == "" {
		return OllamaSummary{}, fmt.Errorf("模型响应格式无效")
	}
	var summary OllamaSummary
	if err := json.Unmarshal([]byte(envelope.Response), &summary); err != nil {
		return OllamaSummary{}, fmt.Errorf("模型摘要 JSON 无效")
	}
	validIDs := map[string]bool{}
	for _, evidence := range episode.Evidence {
		validIDs[evidence.EventID] = true
	}
	for _, evidence := range summary.Evidence {
		if !validIDs[evidence.EventID] {
			return OllamaSummary{}, fmt.Errorf("模型摘要引用了不存在的证据")
		}
	}
	if summary.Title == "" && summary.Objective == "" && summary.Outcome == "" {
		return OllamaSummary{}, fmt.Errorf("模型摘要为空")
	}
	return summary, nil
}
