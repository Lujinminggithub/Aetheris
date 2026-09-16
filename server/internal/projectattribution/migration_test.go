package projectattribution

import (
	"os"
	"strings"
	"testing"
)

func TestAttributionMigrationSupportsVersionedActivationAndResume(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/012_project_attribution.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, required := range []string{
		"project_attributions", "project_attribution_versions", "project_backfill_jobs",
		"rule_version", "active_rule_version", "last_event_id", "needs_review",
		"current_event_projects",
		"ENABLE ROW LEVEL SECURITY",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("attribution migration missing %s", required)
		}
	}
}
