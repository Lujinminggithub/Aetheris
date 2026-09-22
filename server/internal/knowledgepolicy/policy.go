package knowledgepolicy

import (
	"regexp"
	"sort"
	"strings"
)

const (
	DomainDLP              = "dlp"
	DomainEDR              = "edr"
	DomainNetworkTransport = "network_transport"
	DomainRAG              = "rag"
)

var domainTerms = map[string][]string{
	DomainDLP:              {"dlp", "数据防泄漏", "内容保护", "剪贴板", "ocr", "内容识别", "策略阻断"},
	DomainEDR:              {"edr", "端点检测", "minifilter", "pssetcreateprocessnotifyroutine", "wfp", "etw", "进程树", "内核回调", "内核采集", "用户态分析"},
	DomainNetworkTransport: {"fec", "公网丢包", "丢包率", "udp", "媒体队列", "nb_live", "拥塞", "链路质量", "流量限制"},
	DomainRAG:              {"rag", "检索增强", "embedding", "向量检索", "知识库", "qdrant"},
}

var entityTerms = []string{
	"dlp", "edr", "ocr", "minifilter", "wfp", "etw", "fec", "udp", "nb_live.c", "qdrant", "embedding",
	"剪贴板", "文件", "截图", "策略", "阻断", "审计", "进程", "注册表", "网络", "丢包", "媒体队列",
}

var orchestrationMarkers = []string{
	"agent message from", "message type: final_answer", "task name:", "sender: /root", "委派子代理", "可以委派子代理",
	"继续规格实施", "继续实施", "按照规格实施", "which option?", "subagent", "任务路径",
	"根据推荐方案实施", "按照推荐方案实施", "按推荐方案实施", "确认采用这个方案",
}

var wordPattern = regexp.MustCompile(`[a-z][a-z0-9_.]*`)

func Domains(values ...string) []string {
	value := strings.ToLower(strings.Join(values, "\n"))
	result := []string{}
	for domain, terms := range domainTerms {
		for _, term := range terms {
			if strings.Contains(value, term) {
				result = append(result, domain)
				break
			}
		}
	}
	sort.Strings(result)
	return result
}

func Entities(values ...string) []string {
	value := strings.ToLower(strings.Join(values, "\n"))
	seen := map[string]bool{}
	for _, term := range entityTerms {
		if strings.Contains(value, term) {
			seen[term] = true
		}
	}
	for _, token := range wordPattern.FindAllString(value, -1) {
		if len(token) >= 4 && (strings.Contains(token, ".") || strings.Contains(token, "_")) {
			seen[token] = true
		}
	}
	result := make([]string, 0, len(seen))
	for item := range seen {
		result = append(result, item)
	}
	sort.Strings(result)
	return result
}

func IsOrchestration(values ...string) bool {
	value := strings.ToLower(strings.Join(values, "\n"))
	for _, marker := range orchestrationMarkers {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func DomainsCompatible(questionDomains, candidateDomains []string) bool {
	if len(questionDomains) == 0 {
		return true
	}
	if len(candidateDomains) == 0 {
		return false
	}
	allowed := map[string]bool{}
	for _, domain := range questionDomains {
		allowed[domain] = true
	}
	for _, domain := range candidateDomains {
		if !allowed[domain] {
			return false
		}
	}
	return true
}

func ExchangeEligible(question, answer string) bool {
	if strings.TrimSpace(question) == "" || strings.TrimSpace(answer) == "" || IsOrchestration(question, answer) {
		return false
	}
	questionDomains := Domains(question)
	answerDomains := Domains(answer)
	if len(questionDomains) > 0 && !DomainsCompatible(questionDomains, answerDomains) {
		return false
	}
	return true
}

func Contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func CrossDomainTerms(question, answer string) []string {
	questionDomains := Domains(question)
	answerDomains := Domains(answer)
	if DomainsCompatible(questionDomains, answerDomains) {
		return nil
	}
	allowed := map[string]bool{}
	for _, domain := range questionDomains {
		allowed[domain] = true
	}
	result := []string{}
	for _, domain := range answerDomains {
		if !allowed[domain] {
			result = append(result, domain)
		}
	}
	return result
}
