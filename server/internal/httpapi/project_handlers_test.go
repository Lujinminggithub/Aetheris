package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aetheris-dev/aetheris/server/internal/authorization"
	"github.com/aetheris-dev/aetheris/server/internal/projects"
)

type fakeProjectStore struct {
	principal authorization.Principal
	batch     projects.RegistrationBatch
	listed    []projects.ProjectView
}

func (store *fakeProjectStore) Register(_ context.Context, principal authorization.Principal, batch projects.RegistrationBatch) (projects.BatchResult, error) {
	store.principal = principal
	store.batch = batch
	return projects.BatchResult{RegistryRevision: 8, Projects: []projects.RegistrationResult{{
		LocalProjectID: batch.Projects[0].LocalProjectID, LogicalProjectID: "logical-1", DisplayName: batch.Projects[0].DisplayName, Resolution: "new_project",
	}}}, nil
}

func (store *fakeProjectStore) List(_ context.Context, _ string) ([]projects.ProjectView, error) {
	return store.listed, nil
}

func TestDeviceProjectRegistrationUsesCredentialIdentity(t *testing.T) {
	store := &fakeProjectStore{}
	body := `{"projects":[{"local_project_id":"project-1234567890abcdef","display_name":"jtagent","vcs":"git","remote_fingerprint":"hmac-sha256:v1:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","root_fingerprint":"hmac-sha256:v1:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","workspace_kind":"primary","active":true,"key_version":1,"metadata_revision":2}]}`
	request := httptest.NewRequest(http.MethodPut, "/api/v1/device/projects", strings.NewReader(body))
	response := httptest.NewRecorder()
	principal := authorization.Principal{Kind: "device", TenantID: "tenant-1", DeviceID: "device-1", Permissions: map[string]bool{"projects:register": true}}

	serveDeviceProjects(response, request, principal, Dependencies{Projects: store})

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"logical_project_id":"logical-1"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if store.principal.DeviceID != "device-1" || len(store.batch.Projects) != 1 {
		t.Fatalf("registration identity or batch was lost")
	}
}

func TestDeviceProjectRegistrationRejectsNonDeviceAndOversizedBatch(t *testing.T) {
	store := &fakeProjectStore{}
	request := httptest.NewRequest(http.MethodPut, "/api/v1/device/projects", strings.NewReader(`{"projects":[]}`))
	response := httptest.NewRecorder()
	serveDeviceProjects(response, request, authorization.Principal{Kind: "user", TenantID: "tenant-1", Permissions: map[string]bool{"projects:register": true}}, Dependencies{Projects: store})
	if response.Code != http.StatusForbidden {
		t.Fatalf("non-device status=%d", response.Code)
	}
}

func TestAdminProjectListRequiresProjectManagementPermission(t *testing.T) {
	store := &fakeProjectStore{listed: []projects.ProjectView{{ID: "logical-1", DisplayName: "jtagent", LocationCount: 2}}}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/projects", nil)

	forbidden := httptest.NewRecorder()
	serveAdminProjects(forbidden, request, authorization.Principal{Kind: "user", TenantID: "tenant-1", Permissions: map[string]bool{}}, Dependencies{Projects: store})
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("forbidden status=%d", forbidden.Code)
	}

	allowed := httptest.NewRecorder()
	serveAdminProjects(allowed, request, authorization.Principal{Kind: "user", TenantID: "tenant-1", Permissions: map[string]bool{"projects:manage": true}}, Dependencies{Projects: store})
	if allowed.Code != http.StatusOK || !strings.Contains(allowed.Body.String(), `"display_name":"jtagent"`) {
		t.Fatalf("status=%d body=%s", allowed.Code, allowed.Body.String())
	}
}
