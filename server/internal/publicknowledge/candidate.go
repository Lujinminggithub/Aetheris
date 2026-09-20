package publicknowledge

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

var ErrCandidateIneligible = errors.New("private knowledge is not eligible for public review")

func BuildCandidate(source PrivateKnowledge) (Candidate, error) {
	if source.ValidationState != "verified" && source.DecisionState != "accepted" {
		return Candidate{}, ErrCandidateIneligible
	}
	topic, topicReport := SanitizePublicText(source.Topic, source.SensitiveTerms)
	problem, problemReport := SanitizePublicText(source.Problem, source.SensitiveTerms)
	conclusion, conclusionReport := SanitizePublicText(source.Conclusion, source.SensitiveTerms)
	rationale, rationaleReport := SanitizePublicText(source.Rationale, source.SensitiveTerms)
	applicability, applicabilityReport := SanitizePublicText(source.Applicability, source.SensitiveTerms)
	caveats, caveatsReport := SanitizePublicText(source.Caveats, source.SensitiveTerms)
	alternatives, alternativesReport := SanitizePublicText(source.Alternatives, source.SensitiveTerms)
	for _, report := range []SanitizeReport{topicReport, problemReport, conclusionReport, rationaleReport, applicabilityReport, caveatsReport, alternativesReport} {
		if report.Blocked {
			return Candidate{}, ErrCandidateIneligible
		}
	}
	if topic == "" || conclusion == "" {
		return Candidate{}, ErrCandidateIneligible
	}
	canonical := strings.Join([]string{topic, source.KnowledgeType, problem, conclusion, applicability, caveats}, "\x00")
	canonicalHash := hashText(canonical)
	id := "public-knowledge-" + canonicalHash[:32]
	validation := SourceConfirmed
	if source.ValidationState == "verified" {
		validation = EvidenceVerified
	}
	revision := Revision{
		Revision: 1, ProblemPattern: problem, Conclusion: conclusion, Rationale: rationale,
		Applicability: applicability, Caveats: caveats, Alternatives: alternatives,
		ValidationState: validation, AnonymousSourceTenantCount: 1, IndependentSessionCount: 1,
		CanonicalHash: canonicalHash, ExtractorVersion: "deterministic-v1",
		RedactionVersion: "public-v1", ReviewPolicyVersion: "public-v1",
	}
	return Candidate{
		Unit:     Unit{ID: id, CanonicalTopic: topic, KnowledgeType: source.KnowledgeType, PublicationState: PendingReview, CurrentRevision: 1, Revision: revision},
		Revision: revision,
		Source: SourceLink{
			ID:                "public-source-" + hashText(source.SourceTenantID + "\x00" + source.KnowledgeID + "\x00" + id)[:32],
			PublicKnowledgeID: id, PublicRevision: 1, SourceTenantID: source.SourceTenantID,
			SourceKnowledgeID: source.KnowledgeID, SourceRevision: source.Revision, Relation: "supports",
			IndependenceGroup: independenceGroup(source), SourceContentHash: source.SourceContentHash,
			ExternalSourceHash: source.ExternalSourceHash, ImportBatchID: source.ImportBatchID, RedactionState: "passed",
		},
	}, nil
}

func independenceGroup(source PrivateKnowledge) string {
	switch {
	case source.ExternalSourceHash != "":
		return "external:" + hashText(source.ExternalSourceHash)
	case source.ImportBatchID != "":
		return "batch:" + hashText(source.ImportBatchID)
	case source.SourceContentHash != "":
		return "content:" + hashText(source.SourceContentHash)
	default:
		return "session:" + hashText(source.SourceTenantID+"\x00"+source.SessionID)
	}
}

func hashText(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func likelyConflict(leftProblem, leftConclusion, leftApplicability, rightProblem, rightConclusion, rightApplicability string) bool {
	normalize := func(value string) string { return strings.ToLower(strings.Join(strings.Fields(value), "")) }
	if normalize(leftProblem) == "" || normalize(leftProblem) != normalize(rightProblem) || normalize(leftApplicability) != normalize(rightApplicability) {
		return false
	}
	left := normalize(leftConclusion)
	right := normalize(rightConclusion)
	if left == "" || right == "" || left == right {
		return false
	}
	hasNegative := func(value string) bool {
		for _, term := range []string{"不能", "禁止", "不应", "不可", "不支持", "避免", "不得"} {
			if strings.Contains(value, term) {
				return true
			}
		}
		return false
	}
	return hasNegative(left) != hasNegative(right)
}
