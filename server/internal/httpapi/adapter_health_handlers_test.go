package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aetheris-dev/aetheris/server/internal/adapterhealth"
	"github.com/aetheris-dev/aetheris/server/internal/authorization"
)

type fakeAdapterHealthStore struct{ snapshots []adapterhealth.Snapshot }

func (store *fakeAdapterHealthStore) List(_ context.Context, _, _ string) ([]adapterhealth.Snapshot, error) {
	return store.snapshots, nil
}
func (store *fakeAdapterHealthStore) Upsert(_ context.Context, snapshot adapterhealth.Snapshot) error {
	store.snapshots = append(store.snapshots, snapshot)
	return nil
}

func TestDeviceAdapterHealthRejectsOtherDeviceAndStoresSafeSnapshot(t *testing.T) {
	store := &fakeAdapterHealthStore{}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/device/adapter-health", strings.NewReader(`{"snapshots":[{"device_id":"device-1","adapter_id":"browser","state":"error","error_stage":"url_read","error_code":"uia_unavailable"}]}`))
	response := httptest.NewRecorder()
	serveDeviceAdapterHealth(response, request, authorization.Principal{Kind: "device", TenantID: "tenant-1", DeviceID: "device-1", Permissions: map[string]bool{"health:report": true}}, Dependencies{AdapterHealth: store})
	if response.Code != http.StatusAccepted || len(store.snapshots) != 1 {
		t.Fatalf("status=%d snapshots=%d body=%s", response.Code, len(store.snapshots), response.Body.String())
	}
	if store.snapshots[0].TenantID != "tenant-1" || store.snapshots[0].DeviceID != "device-1" {
		t.Fatalf("identity not assigned: %+v", store.snapshots[0])
	}
}

func TestDeviceAdapterHealthRejectsMismatchedIdentity(t *testing.T) {
	response := httptest.NewRecorder()
	serveDeviceAdapterHealth(response, httptest.NewRequest(http.MethodPost, "/api/v1/device/adapter-health", strings.NewReader(`{"snapshots":[{"device_id":"other","adapter_id":"browser","state":"idle"}]}`)), authorization.Principal{Kind: "device", TenantID: "tenant-1", DeviceID: "device-1", Permissions: map[string]bool{"health:report": true}}, Dependencies{AdapterHealth: &fakeAdapterHealthStore{}})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestDeviceAdapterHealthAcceptsVSCodeComponentState(t *testing.T) {
	store := &fakeAdapterHealthStore{}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/device/adapter-health", strings.NewReader(`{"snapshots":[{"adapter_id":"vscode_extension","state":"disabled","component_state":"paused_by_user","component_version":"0.1.0","protocol_version":1,"pending_events":2}]}`))
	response := httptest.NewRecorder()
	serveDeviceAdapterHealth(response, request, authorization.Principal{Kind: "device", TenantID: "tenant-1", DeviceID: "device-1", Permissions: map[string]bool{"health:report": true}}, Dependencies{AdapterHealth: store})
	if response.Code != http.StatusAccepted || len(store.snapshots) != 1 {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if store.snapshots[0].ComponentState != "paused_by_user" || store.snapshots[0].PendingEvents != 2 {
		t.Fatalf("component state not decoded: %+v", store.snapshots[0])
	}
}

func TestDeviceAdapterHealthRejectsUnknownComponentState(t *testing.T) {
	store := &fakeAdapterHealthStore{}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/device/adapter-health", strings.NewReader(`{"snapshots":[{"adapter_id":"vscode_extension","state":"error","component_state":"invented"}]}`))
	response := httptest.NewRecorder()
	serveDeviceAdapterHealth(response, request, authorization.Principal{Kind: "device", TenantID: "tenant-1", DeviceID: "device-1", Permissions: map[string]bool{"health:report": true}}, Dependencies{AdapterHealth: store})
	if response.Code != http.StatusBadRequest || len(store.snapshots) != 0 {
		t.Fatalf("status=%d snapshots=%d body=%s", response.Code, len(store.snapshots), response.Body.String())
	}
}
