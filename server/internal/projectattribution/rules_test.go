package projectattribution

import "testing"

func TestResolveUsesStrictPriorityAndEvidence(t *testing.T) {
	input := Evidence{EventID: "event-1", ProjectLabel: "jtagent", Source: "core.ai.codex"}
	result := Resolve(input, Candidates{
		Exact:   []Candidate{{LogicalProjectID: "logical-exact", LocationID: "location-1"}},
		Label:   []Candidate{{LogicalProjectID: "logical-label"}},
		Session: []Candidate{{LogicalProjectID: "logical-session"}},
	})
	if result.LogicalProjectID != "logical-exact" || result.Method != "exact_binding" || result.Confidence != "high" || result.NeedsReview {
		t.Fatalf("unexpected attribution: %+v", result)
	}
	if result.Evidence["event_id"] != "event-1" || result.Evidence["location_id"] != "location-1" {
		t.Fatalf("missing safe evidence: %+v", result.Evidence)
	}
}

func TestResolveUsesUniqueLabelThenSessionThenTime(t *testing.T) {
	cases := []struct {
		name       string
		candidates Candidates
		method     string
		confidence string
	}{
		{"label", Candidates{Label: []Candidate{{LogicalProjectID: "logical-1"}}}, "safe_label", "high"},
		{"session", Candidates{Session: []Candidate{{LogicalProjectID: "logical-1"}}}, "session_correlation", "medium"},
		{"time", Candidates{Time: []Candidate{{LogicalProjectID: "logical-1"}}}, "time_correlation", "low"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			result := Resolve(Evidence{EventID: "event-1"}, item.candidates)
			if result.LogicalProjectID != "logical-1" || result.Method != item.method || result.Confidence != item.confidence {
				t.Fatalf("unexpected attribution: %+v", result)
			}
		})
	}
}

func TestResolveQuarantinesAmbiguousCandidates(t *testing.T) {
	result := Resolve(Evidence{EventID: "event-1"}, Candidates{Session: []Candidate{
		{LogicalProjectID: "logical-1"}, {LogicalProjectID: "logical-2"},
	}})
	if result.Method != "ambiguous_session" || !result.NeedsReview || result.LogicalProjectID != "" {
		t.Fatalf("ambiguous attribution was published: %+v", result)
	}
}

func TestResolveKeepsToolFallbackUnresolved(t *testing.T) {
	result := Resolve(Evidence{EventID: "event-1", Source: "core.ai.claude_code", ProjectLabel: "Claude Code"}, Candidates{})
	if result.Method != "unresolved" || !result.NeedsReview || result.LogicalProjectID != "" || result.Confidence != "low" {
		t.Fatalf("tool fallback was incorrectly assigned: %+v", result)
	}
}
