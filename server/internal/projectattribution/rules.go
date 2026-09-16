package projectattribution

func Resolve(input Evidence, candidates Candidates) Attribution {
	stages := []struct {
		items      []Candidate
		method     string
		ambiguous  string
		confidence string
	}{
		{candidates.Exact, "exact_binding", "ambiguous_exact", "high"},
		{candidates.Label, "safe_label", "ambiguous_label", "high"},
		{candidates.Session, "session_correlation", "ambiguous_session", "medium"},
		{candidates.Time, "time_correlation", "ambiguous_time", "low"},
	}
	for _, stage := range stages {
		unique := uniqueCandidates(stage.items)
		if len(unique) == 0 {
			continue
		}
		if len(unique) > 1 {
			return Attribution{
				Method: stage.ambiguous, Confidence: "low", NeedsReview: true,
				Evidence: map[string]any{"event_id": input.EventID, "candidate_count": len(unique)},
			}
		}
		candidate := unique[0]
		evidence := map[string]any{"event_id": input.EventID}
		if candidate.LocationID != "" {
			evidence["location_id"] = candidate.LocationID
		}
		return Attribution{
			LogicalProjectID: candidate.LogicalProjectID,
			LocationID:       candidate.LocationID,
			Method:           stage.method,
			Confidence:       stage.confidence,
			Evidence:         evidence,
		}
	}
	return Attribution{
		Method: "unresolved", Confidence: "low", NeedsReview: true,
		Evidence: map[string]any{"event_id": input.EventID, "source": input.Source},
	}
}

func uniqueCandidates(items []Candidate) []Candidate {
	result := []Candidate{}
	seen := map[string]bool{}
	for _, item := range items {
		if item.LogicalProjectID == "" || seen[item.LogicalProjectID] {
			continue
		}
		seen[item.LogicalProjectID] = true
		result = append(result, item)
	}
	return result
}
