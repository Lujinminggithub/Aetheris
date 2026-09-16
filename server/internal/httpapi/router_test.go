package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestCheckModelGatewayReportsProviderReadinessFailure(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/readyz" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"unavailable","provider":"ollama","model":"qwen3:4b-instruct","error":"connection_error"}`))
	}))
	defer upstream.Close()

	status, message, _ := checkModelGateway(context.Background(), upstream.URL)

	if status != "down" {
		t.Fatalf("status = %q", status)
	}
	if message != "Ollama 连接失败" {
		t.Fatalf("message = %q", message)
	}
}

func TestInvalidCredentialsErrorIncludesChineseMessage(t *testing.T) {
	response := httptest.NewRecorder()
	writeInvalidCredentials(response)
	var body map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["message"] != "用户名或密码错误" {
		t.Fatalf("message = %q", body["message"])
	}
}

func TestRootRedirectsToAdminLogin(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	NewRouter(Dependencies{}).ServeHTTP(response, request)
	if response.Code != http.StatusFound {
		t.Fatalf("status = %d", response.Code)
	}
	if location := response.Header().Get("Location"); location != "/admin/" {
		t.Fatalf("location = %q", location)
	}
}

func TestClientDownloadServesOnlyConfiguredInstaller(t *testing.T) {
	installer := t.TempDir() + "/AetherisCore-test.exe"
	if err := os.WriteFile(installer, []byte("installer"), 0o600); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/downloads/client", nil)
	response := httptest.NewRecorder()
	NewRouter(Dependencies{ClientDownloadFile: installer}).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	if disposition := response.Header().Get("Content-Disposition"); disposition != `attachment; filename="AetherisCore-test.exe"` {
		t.Fatalf("content disposition = %q", disposition)
	}
}

func TestDeviceRevokeRequiresAuthentication(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/device/revoke", nil)
	response := httptest.NewRecorder()
	NewRouter(Dependencies{}).ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestDataQualityEndpointsRequireAuthentication(t *testing.T) {
	for _, test := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/admin/data-quality/summary"},
		{http.MethodGet, "/api/v1/admin/data-quality/facts"},
		{http.MethodPost, "/api/v1/admin/data-quality/recompute"},
	} {
		request := httptest.NewRequest(test.method, test.path, nil)
		response := httptest.NewRecorder()
		NewRouter(Dependencies{}).ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s status = %d", test.method, test.path, response.Code)
		}
	}
}

func TestActivityAndRAGEndpointsRequireAuthentication(t *testing.T) {
	for _, test := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/admin/activities"},
		{http.MethodGet, "/api/v1/admin/rag/status"},
		{http.MethodPost, "/api/v1/admin/rag/queries"},
		{http.MethodGet, "/api/v1/admin/rag/queries/rag-1"},
	} {
		response := httptest.NewRecorder()
		NewRouter(Dependencies{}).ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s status = %d", test.method, test.path, response.Code)
		}
	}
}

func TestHealthzDoesNotRequireAuthentication(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()
	NewRouter(Dependencies{}).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestUnknownAdminPathIsNotServed(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/admin/does-not-exist", nil)
	response := httptest.NewRecorder()
	NewRouter(Dependencies{StaticDir: t.TempDir()}).ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d", response.Code)
	}
}
