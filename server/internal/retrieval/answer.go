package retrieval

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/aetheris-dev/aetheris/server/internal/knowledgepolicy"
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
	if answer.Mode == AnalysisMode {
		if len([]rune(strings.TrimSpace(answer.Answer+answer.Details))) < 60 || strings.TrimSpace(answer.Answer) == "分析" || strings.TrimSpace(answer.Answer) == "总结" {
			return fmt.Errorf("分析回答过短或过于泛化，必须结合证据覆盖回答计划中的主题")
		}
		for _, phrase := range []string{"这看起来是一个架构设计任务", "需后续确认", "确认后再拆分", "建议先完成架构设计"} {
			if strings.Contains(answer.Answer+answer.Details, phrase) {
				return fmt.Errorf("回答复述了过程话术，必须提取可执行的技术结论")
			}
		}
	}
	combined := strings.ToLower(answer.Answer + "\n" + answer.Details)
	for _, term := range []string{"qdrant", "embedding", "向量索引", "数据库表", "内部 api", "索引 worker", "检索 worker", "模型网关", "rag", "未连接服务器", "未读取日志", "索引状态"} {
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
				if citationIsVerified(citation) {
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
	if selectedKnowledge && !selectedVerified && strings.Contains(combined, "已验证") {
		return fmt.Errorf("未验证过程知识不能表述为已验证本地结果")
	}
	return nil
}

func citationIsVerified(citation Citation) bool {
	return citation.ValidationState == "verified" || (citation.SourceScope == "platform_public" && citation.ValidationState == "platform_certified")
}

func ValidateAnswerDomain(question string, answer GeneratedAnswer, citations []Citation) error {
	questionDomains := knowledgepolicy.Domains(question)
	if len(questionDomains) == 0 {
		return nil
	}
	if drift := knowledgepolicy.CrossDomainTerms(question, answer.Answer+"\n"+answer.Details); len(drift) > 0 {
		return fmt.Errorf("回答包含跨领域内容: %s", strings.Join(drift, ","))
	}
	for _, citation := range citations {
		if citation.KnowledgeID == "" {
			continue
		}
		citationDomains := citation.Domains
		if len(citationDomains) == 0 {
			citationDomains = knowledgepolicy.Domains(citation.Topic, citation.Excerpt, citation.Applicability)
		}
		if !knowledgepolicy.DomainsCompatible(questionDomains, citationDomains) {
			return fmt.Errorf("回答引用跨领域证据")
		}
	}
	return nil
}
