package processknowledge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProcessKnowledgeMigrationDeclaresVersionedTenantScopedSchema(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", "020_process_knowledge.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, fragment := range []string{
		"CREATE EXTENSION IF NOT EXISTS pg_trgm",
		"superseded_event_inheritance",
		"e.supersedes_event_id",
		"COALESCE(pa.logical_project_id, inherited_pa.logical_project_id)",
		"CREATE TABLE IF NOT EXISTS process_sessions",
		"CREATE TABLE IF NOT EXISTS process_turns",
		"CREATE TABLE IF NOT EXISTS process_knowledge_units",
		"CREATE TABLE IF NOT EXISTS process_knowledge_evidence",
		"CREATE TABLE IF NOT EXISTS process_knowledge_chunks",
		"CREATE TABLE IF NOT EXISTS process_knowledge_jobs",
		"CREATE TABLE IF NOT EXISTS process_knowledge_state",
		"decision_state IN ('proposed','accepted','rejected','superseded')",
		"validation_state IN ('unverified','partially_verified','verified','contradicted')",
		"lifecycle_state IN ('candidate','active','stale','withdrawn')",
		"USING GIN (search_text gin_trgm_ops)",
		"ENABLE ROW LEVEL SECURITY",
		"process_knowledge:read",
		"process_knowledge:manage",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
}
