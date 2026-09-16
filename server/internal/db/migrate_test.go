package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListMigrationFilesOrdersByVersion(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"002_roles.sql", "001_identity.sql", "README.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("-- test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	files, err := listMigrationFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := filepath.Base(files[0]); got != "001_identity.sql" {
		t.Fatalf("first migration = %s", got)
	}
	if got := filepath.Base(files[1]); got != "002_roles.sql" {
		t.Fatalf("second migration = %s", got)
	}
}

func TestListMigrationFilesRejectsDuplicateVersions(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"001_a.sql", "001_b.sql"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("-- test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := listMigrationFiles(dir); err == nil {
		t.Fatal("expected duplicate version error")
	}
}
