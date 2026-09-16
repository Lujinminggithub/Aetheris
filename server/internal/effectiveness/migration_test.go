package effectiveness

import (
	"os"
	"strings"
	"testing"
)

func TestEffectivenessMigrationDeclaresRequiredTablesAndPermissions(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/005_personal_effectiveness.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, required := range []string{
		"subject_effectiveness_daily",
		"effectiveness_recompute_jobs",
		"user_subject_links",
		"effectiveness:read",
		"effectiveness:manage",
		"ENABLE ROW LEVEL SECURITY",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("migration missing %s", required)
		}
	}
}
