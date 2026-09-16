package episodes

import (
	"os"
	"strings"
	"testing"
)

func TestEpisodesMigrationDeclaresEvidenceAndRevisionTables(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/013_work_episodes.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, required := range []string{"work_episodes", "work_episode_actions", "work_episode_decisions", "work_episode_validations", "work_episode_evidence", "revision", "ENABLE ROW LEVEL SECURITY"} {
		if !strings.Contains(sql, required) {
			t.Fatalf("episode migration missing %s", required)
		}
	}
}
