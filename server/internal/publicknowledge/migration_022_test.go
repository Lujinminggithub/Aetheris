package publicknowledge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKnowledgeScopeMigrationAddsStructuredScopeAndWithdrawsGenericV1(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", "022_knowledge_scope_v2.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, fragment := range []string{"domains TEXT[]", "entities TEXT[]", "scope_state", "研发过程知识", "index_status='withdrawn'"} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
}
