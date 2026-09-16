package projects

import (
	"os"
	"strings"
	"testing"
)

func TestProjectRegistryMigrationDeclaresIdentityAndTenantBoundaries(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/011_project_registry.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, required := range []string{
		"logical_projects",
		"project_locations",
		"project_registry_state",
		"remote_fingerprint",
		"UNIQUE (tenant_id, device_id, local_project_id)",
		"ENABLE ROW LEVEL SECURITY",
		"tenant_scope_logical_projects",
		"tenant_scope_project_locations",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("project registry migration missing %s", required)
		}
	}
}
