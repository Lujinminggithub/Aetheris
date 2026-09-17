package retrieval

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

type GeneratedAnswer struct {
	Answer          string     `json:"answer"`
	Mode            AnswerMode `json:"answer_mode"`
	Confidence      string     `json:"confidence"`
	Details         string     `json:"details"`
	CitationNumbers []int      `json:"citation_numbers"`
}

func ParseGeneratedAnswer(raw string) (GeneratedAnswer, error) {
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	var answer GeneratedAnswer
	if err := decoder.Decode(&answer); err != nil {
		return GeneratedAnswer{}, fmt.Errorf("回答 JSON 无效")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return GeneratedAnswer{}, fmt.Errorf("回答 JSON 包含多余内容")
	}
	return answer, nil
}

func ValidateGeneratedAnswer(answer GeneratedAnswer, allowed []Citation) error {
	if !answer.Mode.Valid() {
		return fmt.Errorf("回答模式无效")
	}
	if answer.Confidence != "high" && answer.Confidence != "medium" && answer.Confidence != "low" {
		return fmt.Errorf("回答置信度无效")
	}
	if strings.TrimSpace(answer.Answer) == "" || !utf8.ValidString(answer.Answer) {
		return fmt.Errorf("回答不能为空")
	}
	if answer.Mode == DirectMode {
		if len([]rune(answer.Answer)) > 80 || strings.Count(answer.Answer, "。")+strings.Count(answer.Answer, "！")+strings.Count(answer.Answer, "？") > 2 {
			return fmt.Errorf("直接回答过长")
		}
	}
	if answer.Mode != AnalysisMode && strings.TrimSpace(answer.Details) != "" {
		return fmt.Errorf("非分析回答不得包含详细说明")
	}
	combined := strings.ToLower(answer.Answer + "\n" + answer.Details)
	for _, term := range []string{"qdrant", "embedding", "向量索引", "数据库表", "内部 api", "worker", "模型网关", "rag", "未连接服务器", "未读取日志", "索引状态"} {
		if strings.Contains(combined, term) {
			return fmt.Errorf("回答包含内部技术细节")
		}
	}
	allowedNumbers := map[int]struct{}{}
	for _, citation := range allowed {
		allowedNumbers[citation.Number] = struct{}{}
	}
	if len(allowedNumbers) > 0 && len(answer.CitationNumbers) == 0 {
		return fmt.Errorf("回答缺少证据引用")
	}
	seen := map[int]struct{}{}
	selectedVerified := false
	selectedKnowledge := false
	for _, number := range answer.CitationNumbers {
		if _, ok := allowedNumbers[number]; !ok {
			return fmt.Errorf("回答引用不存在的证据")
		}
		for _, citation := range allowed {
			if citation.Number == number && citation.KnowledgeID != "" {
				selectedKnowledge = true
				if citation.ValidationState == "verified" {
					selectedVerified = true
				}
			}
		}
		if _, ok := seen[number]; ok {
			return fmt.Errorf("回答引用重复")
		}
		seen[number] = struct{}{}
	}
	if answer.Confidence == "high" && selectedKnowledge && !selectedVerified {
		return fmt.Errorf("高置信度回答缺少已验证过程知识")
	}
	return nil
}
