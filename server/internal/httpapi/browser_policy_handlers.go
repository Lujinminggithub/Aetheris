package httpapi

import (
	"errors"
	"net/http"

	"github.com/aetheris-dev/aetheris/server/internal/authorization"
	"github.com/aetheris-dev/aetheris/server/internal/browserpolicy"
	"github.com/aetheris-dev/aetheris/server/internal/httpx"
)

func adminBrowserPolicy(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	withSession(w, r, deps, func(w http.ResponseWriter, r *http.Request, principal authorization.Principal) {
		serveAdminBrowserPolicy(w, r, principal, deps)
	})
}

func serveAdminBrowserPolicy(w http.ResponseWriter, r *http.Request, principal authorization.Principal, deps Dependencies) {
	if err := authorization.Require(principal, "devices:manage", authorization.Scope{TenantID: principal.TenantID}); err != nil {
		httpx.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	if deps.BrowserPolicies == nil {
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "browser_policy_unavailable"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		policy, err := deps.BrowserPolicies.Get(r.Context(), principal.TenantID)
		if err != nil {
			httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "browser_policy_query_failed"})
			return
		}
		httpx.WriteJSON(w, http.StatusOK, policy)
	case http.MethodPut:
		var body struct {
			Enabled        bool     `json:"enabled"`
			AllowedDomains []string `json:"allowed_domains"`
		}
		if err := httpx.ReadJSON(r, 64*1024, &body); err != nil {
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request", "message": "浏览器采集策略格式无效"})
			return
		}
		domains, err := browserpolicy.NormalizeDomains(body.AllowedDomains)
		if err == nil {
			err = browserpolicy.ValidatePolicy(body.Enabled, domains)
		}
		if err != nil {
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_browser_policy", "message": err.Error()})
			return
		}
		policy, err := deps.BrowserPolicies.Update(r.Context(), principal.TenantID, principal.ID, body.Enabled, domains)
		if err != nil {
			if errors.Is(err, browserpolicy.ErrInvalidPolicy) {
				httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_browser_policy", "message": err.Error()})
				return
			}
			httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "browser_policy_update_failed", "message": "浏览器采集策略保存失败"})
			return
		}
		httpx.WriteJSON(w, http.StatusOK, policy)
	default:
		httpx.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
	}
}

func deviceBrowserPolicy(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	principal, ok := devicePrincipal(w, r, deps)
	if !ok {
		return
	}
	serveDeviceBrowserPolicy(w, r, principal, deps)
}

func serveDeviceBrowserPolicy(w http.ResponseWriter, r *http.Request, principal authorization.Principal, deps Dependencies) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if principal.Kind != "device" || deps.BrowserPolicies == nil {
		httpx.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	policy, err := deps.BrowserPolicies.Get(r.Context(), principal.TenantID)
	if err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "browser_policy_query_failed"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, policy)
}
