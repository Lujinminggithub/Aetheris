package episodes

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"
)

const inactivityGap = 30 * time.Minute

func Build(facts []Fact) []Episode {
	ordered := append([]Fact(nil), facts...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].ProjectID != ordered[j].ProjectID {
			return ordered[i].ProjectID < ordered[j].ProjectID
		}
		if ordered[i].DeviceID != ordered[j].DeviceID {
			return ordered[i].DeviceID < ordered[j].DeviceID
		}
		if ordered[i].SessionID != ordered[j].SessionID {
			return ordered[i].SessionID < ordered[j].SessionID
		}
		return ordered[i].OccurredAt.Before(ordered[j].OccurredAt)
	})
	groups := [][]Fact{}
	for _, fact := range ordered {
		if len(groups) == 0 {
			groups = append(groups, []Fact{fact})
			continue
		}
		last := groups[len(groups)-1]
		if last[0].DeviceID == fact.DeviceID && last[0].ProjectID == fact.ProjectID && last[0].SessionID == fact.SessionID && fact.OccurredAt.Sub(last[len(last)-1].OccurredAt) <= inactivityGap {
			groups[len(groups)-1] = append(groups[len(groups)-1], fact)
		} else {
			groups = append(groups, []Fact{fact})
		}
	}
	episodes := make([]Episode, 0, len(groups))
	for _, group := range groups {
		episodes = append(episodes, buildGroup(group))
	}
	return episodes
}

func buildGroup(group []Fact) Episode {
	first, last := group[0], group[len(group)-1]
	episode := Episode{
		EpisodeID: episodeID(first.DeviceID, first.ProjectID, first.SessionID, first.OccurredAt),
		SubjectID: first.SubjectID, DeviceID: first.DeviceID, ProjectID: first.ProjectID, SessionID: first.SessionID,
		StartedAt: first.OccurredAt, EndedAt: last.OccurredAt,
		Actions: []Action{}, Validations: []Validation{}, Evidence: []Evidence{},
		Confidence: "medium", NeedsReview: true,
	}
	seenEvidence := map[string]bool{}
	for _, fact := range group {
		switch {
		case fact.Role == "user" && episode.Objective == "" && strings.TrimSpace(fact.Content) != "":
			episode.Objective = strings.TrimSpace(fact.Content)
			episode.Title = limit(episode.Objective, 80)
			addEvidence(&episode, seenEvidence, fact.EventID, "objective", "用户目标")
		case fact.EventType == "ide.test" || fact.EventType == "ide.build" || fact.EventType == "validation.success" || fact.EventType == "health.check":
			if !successSummary(fact.Summary) {
				continue
			}
			episode.Validations = append(episode.Validations, Validation{EventID: fact.EventID, ValidationType: fact.EventType, Result: "通过", Summary: limit(strings.TrimSpace(fact.Summary), 240), OccurredAt: fact.OccurredAt})
			addEvidence(&episode, seenEvidence, fact.EventID, "validations", "明确成功结果")
		case fact.EventType == "ai.tool_call" || fact.EventType == "terminal.command" || fact.EventType == "application.activity" || strings.HasPrefix(fact.EventType, "ide.") || fact.EventType == "git.commit" || fact.EventType == "git.diff":
			summary := strings.TrimSpace(fact.Summary)
			if summary == "" {
				continue
			}
			episode.Actions = append(episode.Actions, Action{EventID: fact.EventID, Actor: fact.Role, ActionType: fact.EventType, Summary: limit(summary, 240), OccurredAt: fact.OccurredAt})
			addEvidence(&episode, seenEvidence, fact.EventID, "actions", "规范化活动")
		}
	}
	if len(episode.Validations) > 0 {
		episode.Outcome = "验证通过"
		episode.Confidence = "high"
	}
	if episode.Objective == "" {
		episode.NeedsReview = true
	}
	return episode
}

func addEvidence(episode *Episode, seen map[string]bool, eventID, section, reason string) {
	if eventID == "" || seen[eventID+":"+section] {
		return
	}
	seen[eventID+":"+section] = true
	episode.Evidence = append(episode.Evidence, Evidence{EventID: eventID, Section: section, Reason: reason})
}

func successSummary(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.Contains(value, "通过") || strings.Contains(value, "成功") || strings.Contains(value, "pass") || strings.Contains(value, "success")
}

func episodeID(deviceID, projectID, sessionID string, at time.Time) string {
	digest := sha256.Sum256([]byte(deviceID + "\x00" + projectID + "\x00" + sessionID + "\x00" + at.UTC().Format(time.RFC3339Nano)))
	return "episode-" + hex.EncodeToString(digest[:])[:24]
}

func limit(value string, size int) string {
	characters := []rune(value)
	if len(characters) > size {
		return string(characters[:size])
	}
	return value
}
