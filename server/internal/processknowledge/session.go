package processknowledge

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

const inferredSessionGap = 15 * time.Minute

func AssembleSessions(source []SourceTurn) []SessionDraft {
	turns := []SourceTurn{}
	for _, item := range source {
		turns = append(turns, ExpandSourceTurn(item)...)
	}
	sort.SliceStable(turns, func(i, j int) bool {
		if turns[i].OccurredAt.Equal(turns[j].OccurredAt) {
			return turns[i].EventID < turns[j].EventID
		}
		return turns[i].OccurredAt.Before(turns[j].OccurredAt)
	})
	byKey := map[string]*SessionDraft{}
	ordered := []*SessionDraft{}
	lastInferred := map[string]*SessionDraft{}
	for _, sourceTurn := range turns {
		base := strings.Join([]string{sourceTurn.TenantID, sourceTurn.DeviceID, sourceTurn.LogicalProjectID, sourceTurn.AITool}, "\x00")
		var key, method, confidence, sourceSessionID string
		if sourceTurn.SessionID != "" {
			key = base + "\x00exact\x00" + sourceTurn.SessionID
			method, confidence, sourceSessionID = "exact", "high", sourceTurn.SessionID
		} else if previous := lastInferred[base]; previous != nil && sourceTurn.OccurredAt.Sub(previous.EndedAt) <= inferredSessionGap {
			key = previous.ID
			method, confidence = "inferred_time", "low"
		} else {
			key = stableID("session", base, sourceTurn.EventID, sourceTurn.OccurredAt.UTC().Format(time.RFC3339Nano))
			method, confidence = "inferred_time", "low"
		}
		session := byKey[key]
		if session == nil {
			id := key
			if sourceTurn.SessionID != "" {
				id = stableID("session", base, sourceTurn.SessionID)
			}
			session = &SessionDraft{ID: id, TenantID: sourceTurn.TenantID, SubjectID: sourceTurn.SubjectID, DeviceID: sourceTurn.DeviceID, LogicalProjectID: sourceTurn.LogicalProjectID, AITool: sourceTurn.AITool, SourceSessionID: sourceSessionID, AssociationMethod: method, AssociationConfidence: confidence, StartedAt: sourceTurn.OccurredAt, EndedAt: sourceTurn.OccurredAt}
			byKey[key] = session
			if sourceTurn.SessionID == "" {
				byKey[id] = session
			}
			ordered = append(ordered, session)
		}
		turnID := stableID("turn", session.ID, sourceTurn.EventID, sourceTurn.SourceKey)
		session.Turns = append(session.Turns, TurnDraft{ID: turnID, Sequence: len(session.Turns), Kind: ClassifyTurn(sourceTurn), Source: sourceTurn})
		if sourceTurn.OccurredAt.Before(session.StartedAt) {
			session.StartedAt = sourceTurn.OccurredAt
		}
		if sourceTurn.OccurredAt.After(session.EndedAt) {
			session.EndedAt = sourceTurn.OccurredAt
		}
		if sourceTurn.SessionID == "" {
			lastInferred[base] = session
		}
	}
	result := make([]SessionDraft, len(ordered))
	for index, item := range ordered {
		result[index] = *item
	}
	return result
}

func stableID(prefix string, values ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(values, "\x00")))
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(digest[:16]))
}
