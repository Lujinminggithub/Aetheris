package publicknowledge

import (
	"regexp"
	"sort"
	"strings"
)

type SanitizeReport struct {
	Rules            []string `json:"rules"`
	ReplacementCount int      `json:"replacement_count"`
	Blocked          bool     `json:"blocked"`
}

var publicCodeBlock = regexp.MustCompile("(?s)```.*?```")
var publicWindowsPath = regexp.MustCompile(`(?i)\b[a-z]:\\[^\r\n,，。；;]+`)
var publicHomePath = regexp.MustCompile(`(?i)(?:/home/|/Users/)[^/\s]+(?:/[^\s,，。；;]+)*`)
var publicEmail = regexp.MustCompile(`\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`)
var publicInternalURL = regexp.MustCompile(`(?i)https?://(?:localhost|127(?:\.\d{1,3}){3}|10(?:\.\d{1,3}){3}|192\.168(?:\.\d{1,3}){2}|172\.(?:1[6-9]|2\d|3[01])(?:\.\d{1,3}){2})[^\s]*`)
var publicSecret = regexp.MustCompile(`(?i)\b(?:bearer\s+|api[_-]?key\s*[:=]\s*|token\s*[:=]\s*|secret\s*[:=]\s*|password\s*[:=]\s*)[^\s,，。；;]+`)
var publicPrivateKey = regexp.MustCompile(`(?s)-----BEGIN [^-]+ PRIVATE KEY-----.*?-----END [^-]+ PRIVATE KEY-----`)

func SanitizePublicText(value string, sensitiveTerms []string) (string, SanitizeReport) {
	rules := map[string]int{}
	replace := func(pattern *regexp.Regexp, input, replacement, rule string) string {
		matches := pattern.FindAllStringIndex(input, -1)
		if len(matches) > 0 {
			rules[rule] += len(matches)
			return pattern.ReplaceAllString(input, replacement)
		}
		return input
	}
	result := value
	result = replace(publicPrivateKey, result, "[已移除私钥]", "private_key")
	result = replace(publicCodeBlock, result, "[已移除代码]", "code_block")
	result = replace(publicSecret, result, "[已移除凭据]", "credential")
	result = replace(publicEmail, result, "[已移除邮箱]", "email")
	result = replace(publicInternalURL, result, "[已移除内部地址]", "internal_url")
	result = replace(publicWindowsPath, result, "[已移除本机路径]", "windows_path")
	result = replace(publicHomePath, result, "[已移除本机路径]", "home_path")
	for _, term := range sensitiveTerms {
		term = strings.TrimSpace(term)
		if len([]rune(term)) < 2 {
			continue
		}
		pattern := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(term))
		result = replace(pattern, result, "[已移除专有标识]", "tenant_term")
	}
	result = strings.TrimSpace(result)
	report := SanitizeReport{Blocked: strings.Contains(result, "-----BEGIN") || strings.Contains(strings.ToLower(result), "bearer ")}
	for rule, count := range rules {
		report.Rules = append(report.Rules, rule)
		report.ReplacementCount += count
	}
	sort.Strings(report.Rules)
	return result, report
}
