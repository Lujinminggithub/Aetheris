package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aetheris-dev/aetheris/server/internal/authorization"
	"github.com/aetheris-dev/aetheris/server/internal/browserpolicy"
)

type fakeBrowserPolicyStore struct {
	policy browserpolicy.Policy
	actor  string
}

func (store *fakeBrowserPolicyStore) Get(_ context.Context, _ string) (browserpolicy.Policy, error) {
	return store.policy, nil
}

func (store *fakeBrowserPolicyStore) Update(_ context.Context, _ string, actor string, enabled bool, domains []string) (browserpolicy.Policy, error) {
	store.actor = actor
	store.policy = browserpolicy.Policy{Enabled: enabled, AllowedDomains: domains, Revision: 2, UpdatedAt: time.Now().UTC()}
	return store.policy, nil
}

func TestServeAdminBrowserPolicyUpdatesNormalizedPolicy(t *testing.T) {
	store := &fakeBrowserPolicyStore{}
	request := httptest.NewRequest(http.MethodPut, "/api/v1/admin/browser-policy", strings.NewReader(`{"enabled":true,"allowed_domains":["HTTPS://Docs.Example.com/path","docs.example.com."]}`))
	response := httptest.NewRecorder()
	principal := authorization.Principal{Kind: "user", ID: "admin-1", TenantID: "tenant-1", Permissions: map[string]bool{"devices:manage": true}}

	serveAdminBrowserPolicy(response, request, principal, Dependencies{BrowserPolicies: store})

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var policy browserpolicy.Policy
	if err := json.Unmarshal(response.Body.Bytes(), &policy); err != nil {
		t.Fatal(err)
	}
	if store.actor != "admin-1" || len(policy.AllowedDomains) != 1 || policy.AllowedDomains[0] != "docs.example.com" {
		t.Fatalf("policy=%#v actor=%q", policy, store.actor)
	}
}

func TestServeDeviceBrowserPolicyReturnsTenantPolicy(t *testing.T) {
	store := &fakeBrowserPolicyStore{policy: browserpolicy.Policy{Enabled: true, AllowedDomains: []string{"docs.example.com"}, Revision: 7}}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/device/browser-policy", nil)
	response := httptest.NewRecorder()

	serveDeviceBrowserPolicy(response, request, authorization.Principal{Kind: "device", TenantID: "tenant-1"}, Dependencies{BrowserPolicies: store})

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"revision":7`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestServeAdminBrowserPolicyRejectsEnabledEmptyAllowlist(t *testing.T) {
	request := httptest.NewRequest(http.MethodPut, "/api/v1/admin/browser-policy", strings.NewReader(`{"enabled":true,"allowed_domains":[]}`))
	response := httptest.NewRecorder()
	principal := authorization.Principal{Kind: "user", ID: "admin-1", TenantID: "tenant-1", Permissions: map[string]bool{"devices:manage": true}}

	serveAdminBrowserPolicy(response, request, principal, Dependencies{BrowserPolicies: &fakeBrowserPolicyStore{}})

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
