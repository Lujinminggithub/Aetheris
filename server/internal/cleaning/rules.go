package cleaning

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aetheris-dev/aetheris/server/internal/activities"
)

func NormalizeEvents(events []RawEvidence, ruleVersion int) []Fact {
	sort.SliceStable(events, func(i, j int) bool { return events[i].IngestedAt.Before(events[j].IngestedAt) })
	facts := make([]Fact, 0, len(events))
	terminals := make([]RawEvidence, 0)
	for _, event := range events {
		switch event.EventType {
		case "ai.message":
			role := normalizeRole(text(event.Payload["role"]))
			origin := "unknown"
			if role == "user" {
				origin = "human"
			} else if role != "unknown" {
				origin = "ai"
			}
			facts = append(facts, newFact(event, ruleVersion, "ai_interaction", origin, role, "accepted", "high", false, []string{"structured_ai_message"}))
		case "ai.tool_call":
			fact := newFact(event, ruleVersion, "terminal_operation", "ai", "tool", "accepted", "high", false, []string{"structured_ai_tool_call"})
			fact.AITool, fact.CommandType = text(event.Payload["tool"]), text(event.Payload["command_type"])
			fact.CommandSummary, fact.CommandHash = text(event.Payload["command_summary"]), text(event.Payload["command_hash"])
			if text(event.Payload["tool_call_type"]) == "automation" {
				fact.QualityState, fact.Confidence, fact.ExcludedFromEffectiveness = "quarantined", "low", true
				fact.ReasonCodes = append(fact.ReasonCodes, "unresolved_automation_tool_call")
			}
			facts = append(facts, fact)
		case "ai.search_query", "ai.search_result", "ai.tool_result":
			facts = append(facts, newFact(event, ruleVersion, "ai_interaction", "ai", "tool", "accepted", "high", false, []string{"structured_ai_process"}))
		case "ai.reasoning_summary":
			facts = append(facts, newFact(event, ruleVersion, "ai_interaction", "ai", "assistant", "accepted", "high", false, []string{"structured_ai_reasoning_summary"}))
		case "terminal.command":
			terminals = append(terminals, event)
		case "application.activity":
			facts = append(facts, newFact(event, ruleVersion, "activity", "human", "unknown", "accepted", "low", false, []string{"structured_activity", "ocr_fallback"}))
		default:
			if activities.Supported(event.EventType, event.Source) {
				origin := "unknown"
				if strings.HasPrefix(event.EventType, "ide.") && strings.Contains(event.Source, "vscode") {
					origin = "human"
					if event.EventType == "ide.extension_changed" {
						origin = "system"
					}
				}
				fact := newFact(event, ruleVersion, "activity", origin, "unknown", "accepted", "high", false, []string{"structured_activity"})
				if event.EventType == "ide.file_saved" {
					fact.ActivityType = "delivery"
				}
				facts = append(facts, fact)
			}
		}
	}
	for index := 0; index < len(terminals); {
		event := terminals[index]
		command := strings.TrimSpace(text(event.Payload["command"]))
		if hasLineContinuation(command) {
			sources := []string{event.EventID}
			parts := []string{strings.TrimSpace(strings.TrimSuffix(command, "`"))}
			end := index + 1
			for end < len(terminals) && sameScope(event, terminals[end]) && terminals[end].IngestedAt.Sub(event.IngestedAt) <= 5*time.Second {
				next := strings.TrimSpace(text(terminals[end].Payload["command"]))
				continued := hasLineContinuation(next)
				parts = append(parts, strings.TrimSpace(strings.TrimSuffix(next, "`")))
				sources = append(sources, terminals[end].EventID)
				end++
				if !continued {
					break
				}
			}
			fact := terminalFact(event, strings.Join(parts, " "), ruleVersion)
			fact.SourceEventIDs, fact.QualityState, fact.Confidence = sources, "merged", "high"
			fact.MergeMethod = "explicit_backtick_join"
			fact.ReasonCodes, fact.FactID = []string{"explicit_backtick_join"}, factID(fact.FactType, sources)
			facts = append(facts, fact)
			index = end
			continue
		}
		if isParameterFragment(command) {
			if index+1 < len(terminals) && sameScope(event, terminals[index+1]) && terminals[index+1].IngestedAt.Sub(event.IngestedAt) <= 5*time.Second {
				primary := strings.TrimSpace(text(terminals[index+1].Payload["command"]))
				if !isParameterFragment(primary) {
					sources := []string{event.EventID, terminals[index+1].EventID}
					fact := terminalFact(event, primary+" "+command, ruleVersion)
					fact.SourceEventIDs, fact.QualityState, fact.Confidence = sources, "merged", "medium"
					fact.MergeMethod = "inferred_parameter_join"
					fact.ReasonCodes, fact.FactID = []string{"orphan_parameter_fragment", "inferred_parameter_join", "inferred_reordered_join"}, factID(fact.FactType, sources)
					facts = append(facts, fact)
					index += 2
					continue
				}
			}
			fact := terminalFact(event, command, ruleVersion)
			fact.FactType, fact.QualityState, fact.Confidence, fact.ExcludedFromEffectiveness = "command_fragment", "quarantined", "low", true
			fact.ReasonCodes = []string{"orphan_parameter_fragment"}
			facts = append(facts, fact)
			index++
			continue
		}
		if index+1 < len(terminals) && sameScope(event, terminals[index+1]) && terminals[index+1].IngestedAt.Sub(event.IngestedAt) <= 5*time.Second {
			fragment := strings.TrimSpace(text(terminals[index+1].Payload["command"]))
			if isParameterFragment(fragment) {
				if index+2 < len(terminals) && sameScope(terminals[index+1], terminals[index+2]) && terminals[index+2].IngestedAt.Sub(terminals[index+1].IngestedAt) <= 5*time.Second && !isParameterFragment(strings.TrimSpace(text(terminals[index+2].Payload["command"]))) {
					facts = append(facts, terminalFact(event, command, ruleVersion))
					index++
					continue
				}
				sources := []string{event.EventID, terminals[index+1].EventID}
				fact := terminalFact(event, command+" "+fragment, ruleVersion)
				fact.SourceEventIDs, fact.QualityState, fact.Confidence = sources, "merged", "medium"
				fact.MergeMethod = "inferred_parameter_join"
				fact.ReasonCodes, fact.FactID = []string{"orphan_parameter_fragment", "inferred_parameter_join"}, factID(fact.FactType, sources)
				facts = append(facts, fact)
				index += 2
				continue
			}
		}
		facts = append(facts, terminalFact(event, command, ruleVersion))
		index++
	}
	return mergeAITerminalFacts(facts)
}

func terminalFact(event RawEvidence, command string, version int) Fact {
	fact := newFact(event, version, "terminal_operation", defaultText(event.Payload["actor_origin"], "unknown"), "unknown", "accepted", "medium", boolValue(event.Payload["excluded_from_effectiveness"]), []string{"terminal_evidence"})
	fact.CommandText = command
	fact.CommandType, fact.CommandSummary, fact.CommandHash = text(event.Payload["command_type"]), text(event.Payload["command_summary"]), text(event.Payload["command_hash"])
	fact.MergeMethod = defaultText(event.Payload["merge_method"], "none")
	if fact.ExcludedFromEffectiveness || text(event.Payload["quality_state"]) == "quarantined" {
		fact.FactType, fact.QualityState, fact.Confidence = "command_fragment", "quarantined", "low"
	}
	return fact
}

func newFact(event RawEvidence, version int, factType, origin, role, quality, confidence string, excluded bool, reasons []string) Fact {
	return Fact{FactID: factID(factType, []string{event.EventID}), TenantID: event.TenantID, SubjectID: event.SubjectID, DeviceID: event.DeviceID, ProjectID: event.ProjectID, RuleVersion: version, OccurredAt: event.OccurredAt, IngestedAt: event.IngestedAt, FactType: factType, EventType: event.EventType, Source: event.Source, ActivityType: activities.Classify(event.EventType, event.Source), ActorOrigin: origin, MessageRole: role, AITool: text(event.Payload["tool"]), QualityState: quality, Confidence: confidence, MergeMethod: "none", ReasonCodes: reasons, SourceEventIDs: []string{event.EventID}, CanonicalEventID: event.EventID, ExcludedFromEffectiveness: excluded}
}

func mergeAITerminalFacts(facts []Fact) []Fact {
	removed := map[int]bool{}
	for aiIndex := range facts {
		ai := &facts[aiIndex]
		if ai.ActorOrigin != "ai" || ai.MessageRole != "tool" || ai.CommandHash == "" || ai.ExcludedFromEffectiveness {
			continue
		}
		for terminalIndex := range facts {
			terminal := &facts[terminalIndex]
			if removed[terminalIndex] || terminalIndex == aiIndex || terminal.FactType != "terminal_operation" || terminal.MessageRole == "tool" || terminal.CommandHash != ai.CommandHash || terminal.DeviceID != ai.DeviceID {
				continue
			}
			occurredMatch := terminal.ProjectID == ai.ProjectID && within(ai.OccurredAt, terminal.OccurredAt, 30*time.Second)
			ingestedMatch := within(ai.IngestedAt, terminal.IngestedAt, 30*time.Minute)
			if !occurredMatch && !ingestedMatch {
				continue
			}
			ai.SourceEventIDs = append(ai.SourceEventIDs, terminal.SourceEventIDs...)
			ai.QualityState, ai.ReasonCodes = "merged", append(ai.ReasonCodes, "ai_terminal_hash_match", "duplicate_evidence")
			ai.MergeMethod = "ai_terminal_hash_match"
			if terminal.ProjectID != ai.ProjectID {
				ai.Confidence = "medium"
				ai.ReasonCodes = append(ai.ReasonCodes, "project_inferred_from_ai")
			}
			ai.FactID = factID(ai.FactType, ai.SourceEventIDs)
			removed[terminalIndex] = true
			break
		}
	}
	result := make([]Fact, 0, len(facts))
	for index, fact := range facts {
		if !removed[index] {
			result = append(result, fact)
		}
	}
	return result
}

func within(left, right time.Time, window time.Duration) bool {
	if left.IsZero() || right.IsZero() {
		return false
	}
	delta := left.Sub(right)
	return delta >= -window && delta <= window
}

func factID(factType string, sourceIDs []string) string {
	copyIDs := append([]string(nil), sourceIDs...)
	sort.Strings(copyIDs)
	digest := sha256.Sum256([]byte(factType + "\x00" + strings.Join(copyIDs, "\x00")))
	return "fact-" + hex.EncodeToString(digest[:16])
}

func sameScope(left, right RawEvidence) bool {
	return left.DeviceID == right.DeviceID && left.ProjectID == right.ProjectID
}
func isParameterFragment(command string) bool {
	return strings.HasPrefix(strings.TrimSpace(command), "-")
}
func hasLineContinuation(command string) bool {
	trimmed := strings.TrimSpace(command)
	count := 0
	for index := len(trimmed) - 1; index >= 0 && trimmed[index] == '`'; index-- {
		count++
	}
	return count%2 == 1
}
func text(value any) string { result, _ := value.(string); return result }
func defaultText(value any, fallback string) string {
	if result := text(value); result != "" {
		return result
	}
	return fallback
}
func boolValue(value any) bool { result, _ := value.(bool); return result }
func normalizeRole(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "user", "assistant", "system", "tool":
		return value
	default:
		return "unknown"
	}
}

func Counts(facts []Fact) (merged, quarantined int) {
	for _, fact := range facts {
		if fact.QualityState == "merged" {
			merged++
		}
		if fact.QualityState == "quarantined" {
			quarantined++
		}
	}
	return
}

func SafeCommandDisplay(fact Fact) string {
	if fact.ActorOrigin == "ai" {
		return fact.CommandSummary
	}
	if fact.CommandText != "" {
		return fact.CommandText
	}
	return fact.CommandSummary
}
func ValidateRange(from, to time.Time) error {
	if to.Before(from) || int(to.Sub(from).Hours()/24)+1 > 90 {
		return fmt.Errorf("日期范围必须为 1 到 90 天")
	}
	return nil
}
