package processknowledge

import (
	"regexp"
	"strings"

	"github.com/aetheris-dev/aetheris/server/internal/knowledgepolicy"
)

type ClaimDraft struct {
	ID                string
	ParentKnowledgeID string
	ParentRevision    int
	Sequence          int
	Domain            string
	Entities          []string
	Problem           string
	Claim             string
	Applicability     string
	ValidationState   string
	EvidenceIDs       []string
}

var (
	claimLine       = regexp.MustCompile(`(?m)^(?:[-*]\s+|\d+[.)]\s+)?(.{8,600})$`)
	claimHexDigest  = regexp.MustCompile(`(?i)^[a-f0-9]{32,}$`)
	claimSourcePath = regexp.MustCompile(`(?i)^(?:[a-z0-9_.-]+/)+[a-z0-9_.-]+$`)
	claimLocalLink  = regexp.MustCompile(`(?i)\]\(<?/?(?:[a-z]:[/\\]|/)[^)]+\)`)
	claimStatusLine = regexp.MustCompile(`^(?:[^，。；]{0,32})(?:检查|测试|构建|编译|打包)(?:全部)?(?:通过|成功|完成)[。.!]?$`)
	claimSizeLine   = regexp.MustCompile(`^(?:大小|尺寸|文件大小|总大小)\s*[:：]`)
	claimCommand    = regexp.MustCompile(`(?i)^(?:powershell|pwsh|cmd(?:\.exe)?|reg(?:\.exe)?\s+(?:add|delete|query)|git\s+|go\s+(?:test|build)|npm\s+|pnpm\s+|yarn\s+|curl\s+|wget\s+|docker\s+|kubectl\s+)`)
	claimValidation = regexp.MustCompile(`(?:检查|测试|构建|编译|打包|门禁|自检|签名|哈希验证).{0,80}(?:均通过|全部通过|已通过|通过)`)
	claimIdentifier = regexp.MustCompile("(?i)^[a-z0-9_]+[`'、]")
)

func AtomizeKnowledge(unit KnowledgeDraft, revision int, evidenceIDs []string) []ClaimDraft {
	if len(evidenceIDs) == 0 || knowledgepolicy.IsOrchestration(unit.Problem, unit.Conclusion) {
		return nil
	}
	problemDomains := knowledgepolicy.Domains(unit.Problem)
	if len(problemDomains) == 0 {
		return nil
	}
	parts := claimLine.FindAllStringSubmatch(unit.Conclusion, -1)
	if len(parts) == 0 {
		parts = [][]string{{unit.Conclusion}}
	}
	claims := make([]ClaimDraft, 0, len(parts))
	seen := map[string]bool{}
	for _, match := range parts {
		claim := strings.TrimSpace(match[len(match)-1])
		claim = strings.TrimSpace(strings.Trim(claim, "`"))
		if len([]rune(claim)) < 8 || seen[claim] || knowledgepolicy.IsOrchestration(claim) {
			continue
		}
		lower := strings.ToLower(claim)
		if strings.Contains(lower, "pass") || strings.Contains(lower, "测试通过") || strings.Contains(lower, "构建通过") || strings.Contains(lower, "git commit") || strings.Contains(lower, "head ") || strings.Contains(claim, "请确认") || strings.Contains(claim, "实施计划") || strings.Contains(claim, "[实施计划]") {
			continue
		}
		claimDomains := knowledgepolicy.Domains(claim)
		if len(claimDomains) > 1 || (len(claimDomains) == 1 && !knowledgepolicy.Contains(problemDomains, claimDomains[0])) {
			continue
		}
		if isClaimNoise(claim) {
			continue
		}
		entities := knowledgepolicy.Entities(claim, unit.Applicability)
		domain := ""
		if len(claimDomains) == 1 {
			domain = claimDomains[0]
		} else if len(problemDomains) == 1 {
			domain = problemDomains[0]
		} else {
			continue
		}
		validation := unit.ValidationState
		id := stableID("claim", unit.ID, string(rune(len(claims))), claim)
		claims = append(claims, ClaimDraft{ID: id, ParentKnowledgeID: unit.ID, ParentRevision: revision, Sequence: len(claims), Domain: domain, Entities: entities, Problem: strings.TrimSpace(unit.Problem), Claim: claim, Applicability: strings.TrimSpace(unit.Applicability), ValidationState: validation, EvidenceIDs: append([]string(nil), evidenceIDs...)})
		seen[claim] = true
	}
	return claims
}

func isClaimNoise(claim string) bool {
	if len([]rune(claim)) < 12 || strings.HasPrefix(claim, "**") || strings.HasPrefix(claim, "|") || strings.HasPrefix(claim, "{") || strings.HasPrefix(claim, "[") || strings.HasPrefix(claim, "->") || strings.HasPrefix(claim, "<-") || strings.HasSuffix(claim, "：") || strings.HasSuffix(claim, ":") {
		return true
	}
	if strings.Contains(claim, "SHA-256：") || strings.Contains(claim, "http://") || strings.Contains(claim, "https://") || strings.Contains(claim, "warnings=") || strings.HasPrefix(claim, "成功：") || strings.HasPrefix(claim, "新增") || strings.HasPrefix(claim, "新安装包") {
		return true
	}
	if strings.HasPrefix(claim, "这属于") && (strings.Contains(claim, "任务") || strings.Contains(claim, "子系统")) {
		return true
	}
	if claimValidation.MatchString(claim) && !strings.Contains(claim, "通过后") {
		return true
	}
	return claimHexDigest.MatchString(claim) || claimSourcePath.MatchString(claim) || claimLocalLink.MatchString(claim) || claimStatusLine.MatchString(claim) || claimSizeLine.MatchString(claim) || claimCommand.MatchString(claim) || claimIdentifier.MatchString(claim)
}
