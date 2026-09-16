package cleaning

import (
	"os"
	"strings"
	"testing"
)

func TestMigrationCreatesVersionedTenantScopedCleaningTables(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/007_clean_event_facts.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, required := range []string{"clean_event_facts", "cleaning_jobs", "PRIMARY KEY (tenant_id, fact_id, rule_version)", "ENABLE ROW LEVEL SECURITY", "source_event_ids"} {
		if !strings.Contains(sql, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
}

func TestUnifiedActivityAndRetrievalMigrationsContainRequiredGuards(t *testing.T) {
	activity, err := os.ReadFile("../../migrations/008_unified_activity_rag.sql")
	if err != nil { t.Fatal(err) }
	supersession, err := os.ReadFile("../../migrations/009_event_supersession.sql")
	if err != nil { t.Fatal(err) }
	for _, required := range []string{"activity_type", "retrieval_documents", "retrieval_query_jobs", "ENABLE ROW LEVEL SECURITY", "retrieval:query"} {
		if !strings.Contains(string(activity), required) { t.Fatalf("migration 008 missing %q", required) }
	}
	if !strings.Contains(string(supersession), "events_supersedes_idx") { t.Fatal("migration 009 missing supersession index") }
}
