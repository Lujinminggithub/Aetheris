package publicknowledge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicKnowledgeMigrationDefinesGovernedSharedDomain(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", "021_public_knowledge.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS public_knowledge_units",
		"CREATE TABLE IF NOT EXISTS public_knowledge_revisions",
		"CREATE TABLE IF NOT EXISTS public_knowledge_sources",
		"CREATE TABLE IF NOT EXISTS public_knowledge_reviews",
		"CREATE TABLE IF NOT EXISTS public_knowledge_conflicts",
		"CREATE TABLE IF NOT EXISTS public_knowledge_chunks",
		"CREATE TABLE IF NOT EXISTS public_knowledge_jobs",
		"knowledge:confirm_source",
		"knowledge:verify",
		"knowledge:certify_public",
		"knowledge:withdraw_public",
		"knowledge:diagnose",
		"source_tenant_id",
		"anonymous_source_tenant_count",
		"platform_certified",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
}

func TestPublicKnowledgeMigrationKeepsSourceBridgeOutOfPublicProjection(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", "021_public_knowledge.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	viewStart := strings.Index(sql, "CREATE OR REPLACE VIEW published_public_knowledge")
	if viewStart < 0 {
		t.Fatal("published public knowledge view is missing")
	}
	viewEnd := strings.Index(sql[viewStart:], ";")
	if viewEnd < 0 {
		t.Fatal("published public knowledge view is incomplete")
	}
	view := sql[viewStart : viewStart+viewEnd]
	for _, forbidden := range []string{"source_tenant_id", "source_knowledge_id", "source_event", "device_id", "logical_project_id"} {
		if strings.Contains(view, forbidden) {
			t.Fatalf("public view leaks %q", forbidden)
		}
	}
}

func TestProcessKnowledgeConstraintsAcceptObservableAIProcessEvidence(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", "021_public_knowledge.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, value := range []string{"search_query", "search_result", "reasoning_summary", "external_source", "tool_result", "analysis"} {
		if !strings.Contains(sql, "'"+value+"'") {
			t.Fatalf("migration does not allow %s", value)
		}
	}
}
