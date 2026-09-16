package adapterhealth

import (
	"os"
	"strings"
	"testing"
)

func TestSafeSnapshotRemovesTenantIdentityOnly(t *testing.T) {
	snapshot := Snapshot{TenantID: "tenant-1", DeviceID: "device-1", AdapterID: "browser", State: "error", ErrorCode: "uia_unavailable", ErrorStage: "url_read"}
	safe := SafeSnapshot(snapshot)
	if safe.TenantID != "" || safe.DeviceID != "device-1" || safe.ErrorCode != "uia_unavailable" || safe.ErrorStage != "url_read" {
		t.Fatalf("unsafe snapshot projection: %+v", safe)
	}
}

func TestSafeSnapshotKeepsVSCodeComponentState(t *testing.T) {
	snapshot := Snapshot{TenantID: "tenant-1", DeviceID: "device-1", AdapterID: "vscode_extension", State: "disabled", ComponentState: "paused_by_user", ComponentVersion: "0.1.0", ProtocolVersion: 1, PendingEvents: 2}
	safe := SafeSnapshot(snapshot)
	if safe.TenantID != "" || safe.ComponentState != "paused_by_user" || safe.ComponentVersion != "0.1.0" || safe.ProtocolVersion != 1 || safe.PendingEvents != 2 {
		t.Fatalf("component state lost: %+v", safe)
	}
}

func TestVSCodeComponentMigrationDeclaresFields(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/016_vscode_component_state.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, field := range []string{"component_state", "component_version", "protocol_version", "last_component_heartbeat_at", "pending_events"} {
		if !strings.Contains(text, field) {
			t.Fatalf("migration missing %s", field)
		}
	}
}
