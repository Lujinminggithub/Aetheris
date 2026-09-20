package publicknowledge

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPublicListQueryDoesNotJoinPrivateSourceBridge(t *testing.T) {
	for _, query := range []string{listUnitsSQL, countUnitsSQL, getUnitSQL} {
		if strings.Contains(query, "public_knowledge_sources") || strings.Contains(query, "source_tenant_id") || strings.Contains(query, "source_knowledge_id") {
			t.Fatalf("public query exposes the private source bridge: %s", query)
		}
	}
}

func TestPublicUnitJSONHasNoCrossTenantSourceFields(t *testing.T) {
	raw, err := json.Marshal(Unit{ID: "public-1", Revision: Revision{AnonymousSourceTenantCount: 3}})
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(raw)
	for _, forbidden := range []string{"source_tenant_id", "source_knowledge_id", "logical_project_id", "device_id", "event_id"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("public DTO leaks %s: %s", forbidden, encoded)
		}
	}
}
