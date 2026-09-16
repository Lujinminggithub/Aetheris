package retrieval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStructuredAnswerMigrationDeclaresBackwardCompatibleFields(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", "019_structured_rag_answers.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, fragment := range []string{"answer_mode", "answer_confidence", "answer_details", "citation_numbers", "DEFAULT 'direct'", "DEFAULT 'low'"} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
}
