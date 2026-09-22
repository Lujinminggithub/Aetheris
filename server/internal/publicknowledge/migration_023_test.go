package publicknowledge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClaimsMigrationDefinesAtomicKnowledgeAndRelations(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", "023_process_knowledge_claims.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, fragment := range []string{"process_knowledge_claims", "process_knowledge_claim_relations", "domain TEXT", "entities TEXT[]", "relation_type"} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
}
