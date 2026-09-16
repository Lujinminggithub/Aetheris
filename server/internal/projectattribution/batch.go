package projectattribution

type BatchEvidence struct {
	Evidence   Evidence
	Candidates Candidates
}

func ResolveBatch(rows []BatchEvidence) []Attribution {
	results := make([]Attribution, len(rows))
	sessionCandidates := map[string][]Candidate{}
	for index, row := range rows {
		results[index] = Resolve(row.Evidence, row.Candidates)
		if row.Evidence.SessionID != "" && results[index].LogicalProjectID != "" && results[index].Confidence == "high" {
			sessionCandidates[row.Evidence.SessionID] = append(sessionCandidates[row.Evidence.SessionID], Candidate{
				LogicalProjectID: results[index].LogicalProjectID,
				LocationID:       results[index].LocationID,
			})
		}
	}
	for index, row := range rows {
		if results[index].Method != "unresolved" || row.Evidence.SessionID == "" {
			continue
		}
		row.Candidates.Session = sessionCandidates[row.Evidence.SessionID]
		results[index] = Resolve(row.Evidence, row.Candidates)
	}
	return results
}
