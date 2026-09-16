package httpapi

import (
	"net/http"

	"github.com/aetheris-dev/aetheris/server/internal/authorization"
	"github.com/aetheris-dev/aetheris/server/internal/httpx"
)

func adminApplicationPolicy(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	withSession(w, r, deps, func(w http.ResponseWriter, r *http.Request, principal authorization.Principal) {
		serveAdminApplicationPolicy(w, r, principal, deps)
	})
}

func serveAdminApplicationPolicy(w http.ResponseWriter, r *http.Request, principal authorization.Principal, deps Dependencies) {
	if err := authorization.Require(principal, "devices:manage", authorization.Scope{TenantID: principal.TenantID}); err != nil {
		httpx.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	if deps.ApplicationPolicies == nil {
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "application_policy_unavailable"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		policy, err := deps.ApplicationPolicies.Get(r.Context(), principal.TenantID)
		if err != nil {
			httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "application_policy_query_failed"})
			return
		}
		httpx.WriteJSON(w, http.StatusOK, policy)
	case http.MethodPut:
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if err := httpx.ReadJSON(r, 1024, &body); err != nil {
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request", "message": "应用 OCR 保底策略格式无效"})
			return
		}
		policy, err := deps.ApplicationPolicies.Update(r.Context(), principal.TenantID, principal.ID, body.Enabled)
		if err != nil {
			httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "application_policy_update_failed", "message": "应用 OCR 保底策略保存失败"})
			return
		}
		httpx.WriteJSON(w, http.StatusOK, policy)
	default:
		httpx.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
	}
}

func deviceApplicationPolicy(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	principal, ok := devicePrincipal(w, r, deps)
	if !ok {
		return
	}
	serveDeviceApplicationPolicy(w, r, principal, deps)
}

func serveDeviceApplicationPolicy(w http.ResponseWriter, r *http.Request, principal authorization.Principal, deps Dependencies) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if principal.Kind != "device" || deps.ApplicationPolicies == nil {
		httpx.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	policy, err := deps.ApplicationPolicies.Get(r.Context(), principal.TenantID)
	if err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "application_policy_query_failed"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, policy)
}
