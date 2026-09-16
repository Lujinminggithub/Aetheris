package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aetheris-dev/aetheris/server/internal/applicationpolicy"
	"github.com/aetheris-dev/aetheris/server/internal/authorization"
)

type fakeApplicationPolicyStore struct {
	policy applicationpolicy.Policy
	actor  string
}

func (store *fakeApplicationPolicyStore) Get(context.Context, string) (applicationpolicy.Policy, error) {
	if store.policy.Revision == 0 && !store.policy.Enabled {
		return applicationpolicy.DefaultPolicy(), nil
	}
	return store.policy, nil
}
func (store *fakeApplicationPolicyStore) Update(_ context.Context, _ string, actor string, enabled bool) (applicationpolicy.Policy, error) {
	store.actor = actor
	store.policy = applicationpolicy.Policy{Enabled: enabled, Revision: 2}
	return store.policy, nil
}

func TestAdminApplicationPolicyCanDisableFallback(t *testing.T) {
	store := &fakeApplicationPolicyStore{}
	request := httptest.NewRequest(http.MethodPut, "/api/v1/admin/application-capture-policy", strings.NewReader(`{"enabled":false}`))
	response := httptest.NewRecorder()
	principal := authorization.Principal{Kind: "user", ID: "admin-1", TenantID: "tenant-1", Permissions: map[string]bool{"devices:manage": true}}

	serveAdminApplicationPolicy(response, request, principal, Dependencies{ApplicationPolicies: store})

	if response.Code != http.StatusOK || store.actor != "admin-1" || !strings.Contains(response.Body.String(), `"enabled":false`) {
		t.Fatalf("status=%d actor=%q body=%s", response.Code, store.actor, response.Body.String())
	}
}

func TestDeviceApplicationPolicyIsReadOnly(t *testing.T) {
	store := &fakeApplicationPolicyStore{policy: applicationpolicy.Policy{Enabled: true, Revision: 3}}
	getResponse := httptest.NewRecorder()
	serveDeviceApplicationPolicy(getResponse, httptest.NewRequest(http.MethodGet, "/api/v1/device/application-capture-policy", nil), authorization.Principal{Kind: "device", TenantID: "tenant-1"}, Dependencies{ApplicationPolicies: store})
	if getResponse.Code != http.StatusOK || !strings.Contains(getResponse.Body.String(), `"revision":3`) {
		t.Fatalf("status=%d body=%s", getResponse.Code, getResponse.Body.String())
	}
	putResponse := httptest.NewRecorder()
	serveDeviceApplicationPolicy(putResponse, httptest.NewRequest(http.MethodPut, "/api/v1/device/application-capture-policy", strings.NewReader(`{"enabled":false}`)), authorization.Principal{Kind: "device", TenantID: "tenant-1"}, Dependencies{ApplicationPolicies: store})
	if putResponse.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d", putResponse.Code)
	}
}
