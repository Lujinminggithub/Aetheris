package authorization

import "testing"

func TestDeviceIngestCannotReadEvents(t *testing.T) {
	principal := Principal{Kind: "device", TenantID: "tenant-1", DeviceID: "device-1", Permissions: map[string]bool{"events:ingest": true}}
	if err := Require(principal, "events:ingest", Scope{TenantID: "tenant-1"}); err != nil {
		t.Fatal(err)
	}
	if err := Require(principal, "events:read", Scope{TenantID: "tenant-1"}); err == nil {
		t.Fatal("expected read denial")
	}
}

func TestPrincipalCannotCrossTenant(t *testing.T) {
	principal := Principal{Kind: "user", TenantID: "tenant-1", Permissions: map[string]bool{"events:read": true}}
	if err := Require(principal, "events:read", Scope{TenantID: "tenant-2"}); err == nil {
		t.Fatal("expected tenant denial")
	}
}
