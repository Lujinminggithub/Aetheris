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

var claimLine = regexp.MustCompile(`(?m)^(?:[-*]\s+|\d+[.)]\s+)?(.{8,600})$`)

func AtomizeKnowledge(unit KnowledgeDraft, revision int, evidenceIDs []string) []ClaimDraft {
	if knowledgepolicy.IsOrchestration(unit.Problem, unit.Conclusion) {
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
		domains := knowledgepolicy.Domains(unit.Topic, unit.Problem, claim, unit.Applicability)
		if !knowledgepolicy.DomainsCompatible(problemDomains, domains) {
			continue
		}
		entities := knowledgepolicy.Entities(unit.Topic, claim, unit.Applicability)
		domain := domains[0]
		validation := unit.ValidationState
		if validation == "verified" && len(evidenceIDs) == 0 {
			validation = "partially_verified"
		}
		id := stableID("claim", unit.ID, string(rune(len(claims))), claim)
		claims = append(claims, ClaimDraft{ID: id, ParentKnowledgeID: unit.ID, ParentRevision: revision, Sequence: len(claims), Domain: domain, Entities: entities, Problem: strings.TrimSpace(unit.Problem), Claim: claim, Applicability: strings.TrimSpace(unit.Applicability), ValidationState: validation, EvidenceIDs: append([]string(nil), evidenceIDs...)})
		seen[claim] = true
	}
	return claims
}
