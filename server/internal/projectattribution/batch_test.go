package projectattribution

import "testing"

func TestResolveBatchPropagatesOnlyUniqueSessionProject(t *testing.T) {
	rows := []BatchEvidence{
		{Evidence: Evidence{EventID: "event-1", SessionID: "session-a"}, Candidates: Candidates{Exact: []Candidate{{LogicalProjectID: "logical-1", LocationID: "location-1"}}}},
		{Evidence: Evidence{EventID: "event-2", SessionID: "session-a"}},
		{Evidence: Evidence{EventID: "event-3", SessionID: "session-b"}, Candidates: Candidates{Exact: []Candidate{{LogicalProjectID: "logical-1"}}}},
		{Evidence: Evidence{EventID: "event-4", SessionID: "session-b"}, Candidates: Candidates{Exact: []Candidate{{LogicalProjectID: "logical-2"}}}},
		{Evidence: Evidence{EventID: "event-5", SessionID: "session-b"}},
	}

	results := ResolveBatch(rows)

	if results[1].LogicalProjectID != "logical-1" || results[1].Method != "session_correlation" {
		t.Fatalf("unique session project did not propagate: %+v", results[1])
	}
	if results[4].LogicalProjectID != "" || results[4].Method != "ambiguous_session" || !results[4].NeedsReview {
		t.Fatalf("ambiguous session was incorrectly assigned: %+v", results[4])
	}
}
