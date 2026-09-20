package processknowledge

import (
	"strings"
)

func ExtractKnowledge(session SessionDraft) []KnowledgeDraft {
	result := []KnowledgeDraft{}
	var question *TurnDraft
	constraints := []string{}
	contextEvidence := []EvidenceDraft{}
	rationale := []string{}
	var answer *TurnDraft
	decision := "proposed"
	validation := "unverified"
	flush := func() {
		if question == nil || answer == nil {
			question, answer = nil, nil
			constraints, rationale = nil, nil
			contextEvidence = nil
			decision, validation = "proposed", "unverified"
			return
		}
		knowledgeID := stableID("knowledge", session.ID, question.ID, answer.ID)
		evidence := append([]EvidenceDraft(nil), contextEvidence...)
		evidence = append(evidence,
			newEvidence(knowledgeID, *question, "question", "problem", "supports", "human_intent"),
			newEvidence(knowledgeID, *answer, "answer", "conclusion", "supports", "ai_final_answer"),
		)
		result = append(result, KnowledgeDraft{
			ID: knowledgeID, SessionID: session.ID, LogicalProjectID: session.LogicalProjectID,
			Topic: inferTopic(question.Source.Content, strings.Join(constraints, " ")), KnowledgeType: "implementation_pattern",
			Problem: strings.TrimSpace(question.Source.Content), Intent: strings.TrimSpace(question.Source.Content),
			Constraints: strings.Join(constraints, "\n"), Conclusion: strings.TrimSpace(answer.Source.Content),
			Rationale:     strings.Join(rationale, "\n"),
			Applicability: inferApplicability(question.Source.Content, answer.Source.Content), DecisionState: decision,
			ValidationState: validation, LifecycleState: "active", OccurredAt: question.Source.OccurredAt, Evidence: evidence,
		})
		question, answer = nil, nil
		constraints, rationale = nil, nil
		contextEvidence = nil
		decision, validation = "proposed", "unverified"
	}

	for index := range session.Turns {
		turn := session.Turns[index]
		switch turn.Kind {
		case HumanQuestion, HumanFollowup:
			if answer != nil {
				flush()
			}
			question = &turn
		case HumanConstraint:
			if answer != nil {
				flush()
			}
			if question == nil {
				question = &turn
			}
			constraints = append(constraints, strings.TrimSpace(turn.Source.Content))
			contextEvidence = append(contextEvidence, newEvidence("", turn, "constraint", "constraints", "supports", "human_constraint"))
		case AIFinalAnswer:
			if question != nil {
				answer = &turn
			}
		case SearchQuery:
			if question != nil {
				contextEvidence = append(contextEvidence, newEvidence("", turn, "search", "rationale", "context", "ai_search_query"))
			}
		case SearchResult:
			if question != nil {
				rationale = append(rationale, strings.TrimSpace(turn.Source.Content))
				contextEvidence = append(contextEvidence, newEvidence("", turn, "external_source", "rationale", "supports", "ai_search_result"))
			}
		case ReasoningSummary:
			if question != nil {
				rationale = append(rationale, strings.TrimSpace(turn.Source.Content))
				contextEvidence = append(contextEvidence, newEvidence("", turn, "analysis", "rationale", "context", "explicit_reasoning_summary"))
			}
		case ToolCall:
			if question != nil {
				contextEvidence = append(contextEvidence, newEvidence("", turn, "tool", "rationale", "context", "ai_tool_call"))
			}
		case ToolResult:
			if question != nil {
				rationale = append(rationale, strings.TrimSpace(turn.Source.Content))
				contextEvidence = append(contextEvidence, newEvidence("", turn, "tool_result", "rationale", "context", "ai_tool_result"))
			}
		case HumanConfirmation:
			if answer != nil {
				decision = "accepted"
				contextEvidence = append(contextEvidence, newEvidence("", turn, "confirmation", "conclusion", "supports", "human_confirmation"))
			}
		case HumanRejection:
			if answer != nil {
				decision = "rejected"
				validation = "contradicted"
				contextEvidence = append(contextEvidence, newEvidence("", turn, "rejection", "conclusion", "contradicts", "human_rejection"))
				flush()
			}
		case TestResult, BuildResult, RuntimeValidation:
			if answer != nil {
				if isSuccessfulValidation(turn.Source.Content) {
					validation = "verified"
				} else if validation != "verified" {
					validation = "partially_verified"
				}
				kind := "runtime"
				if turn.Kind == TestResult {
					kind = "test"
				}
				if turn.Kind == BuildResult {
					kind = "build"
				}
				contextEvidence = append(contextEvidence, newEvidence("", turn, kind, "validation", "supports", "execution_result"))
			}
		}
	}
	flush()
	for unitIndex := range result {
		for evidenceIndex := range result[unitIndex].Evidence {
			if result[unitIndex].Evidence[evidenceIndex].ID == "" {
				result[unitIndex].Evidence[evidenceIndex].ID = stableID("evidence", result[unitIndex].ID, result[unitIndex].Evidence[evidenceIndex].TurnID, result[unitIndex].Evidence[evidenceIndex].Kind)
			}
		}
	}
	return result
}

func newEvidence(knowledgeID string, turn TurnDraft, kind, section, relation, reason string) EvidenceDraft {
	id := ""
	if knowledgeID != "" {
		id = stableID("evidence", knowledgeID, turn.ID, kind)
	}
	return EvidenceDraft{ID: id, Kind: kind, EventID: turn.Source.EventID, FactID: turn.Source.FactID, TurnID: turn.ID, Section: section, Relation: relation, ReasonCode: reason}
}

func isSuccessfulValidation(content string) bool {
	value := strings.ToLower(content)
	return containsAny(value, "pass", "passed", "ok", "通过", "成功", "exit 0", "exit=0") && !containsAny(value, "fail", "failed", "失败", "error")
}

func inferTopic(values ...string) string {
	value := strings.ToLower(strings.Join(values, " "))
	switch {
	case containsAny(value, "edr", "端点检测", "minifilter", "wfp"):
		return "Windows EDR"
	case containsAny(value, "dlp", "数据防泄漏"):
		return "DLP"
	case containsAny(value, "rag", "检索增强"):
		return "RAG"
	default:
		return "研发过程知识"
	}
}

func inferApplicability(values ...string) string {
	value := strings.ToLower(strings.Join(values, " "))
	if containsAny(value, "windows", "minifilter", "wfp", "内核") {
		return "Windows 终端与内核/用户态协作场景"
	}
	return "当前逻辑项目对应的技术场景"
}
