package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aetheris-dev/aetheris/server/internal/authorization"
)

func TestProjectIdentityKeyIsReturnedOnlyToAuthorizedDevice(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	deps := Dependencies{ProjectIdentityKey: key, ProjectIdentityKeyVersion: 3}

	allowed := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/device/project-identity-key", nil)
	serveDeviceProjectIdentityKey(allowed, request, authorization.Principal{
		Kind: "device", TenantID: "tenant-1", DeviceID: "device-1",
		Permissions: map[string]bool{"projects:register": true},
	}, deps)
	if allowed.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", allowed.Code, allowed.Body.String())
	}
	var body struct {
		Key     string `json:"key"`
		Version int    `json:"version"`
	}
	if err := json.Unmarshal(allowed.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Key != base64.StdEncoding.EncodeToString(key) || body.Version != 3 {
		t.Fatalf("unexpected response: %+v", body)
	}

	forbidden := httptest.NewRecorder()
	serveDeviceProjectIdentityKey(forbidden, request, authorization.Principal{
		Kind: "user", TenantID: "tenant-1", Permissions: map[string]bool{"projects:register": true},
	}, deps)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("user status = %d", forbidden.Code)
	}
}

func TestProjectIdentityKeyRejectsMissingPermissionAndInvalidConfiguration(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/device/project-identity-key", nil)
	principal := authorization.Principal{Kind: "device", TenantID: "tenant-1", Permissions: map[string]bool{}}

	missingPermission := httptest.NewRecorder()
	serveDeviceProjectIdentityKey(missingPermission, request, principal, Dependencies{ProjectIdentityKey: make([]byte, 32), ProjectIdentityKeyVersion: 1})
	if missingPermission.Code != http.StatusForbidden {
		t.Fatalf("missing permission status = %d", missingPermission.Code)
	}

	principal.Permissions["projects:register"] = true
	invalidConfig := httptest.NewRecorder()
	serveDeviceProjectIdentityKey(invalidConfig, request, principal, Dependencies{ProjectIdentityKey: []byte("short"), ProjectIdentityKeyVersion: 1})
	if invalidConfig.Code != http.StatusServiceUnavailable {
		t.Fatalf("invalid config status = %d", invalidConfig.Code)
	}
}
