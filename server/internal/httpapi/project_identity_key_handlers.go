package httpapi

import (
	"encoding/base64"
	"net/http"

	"github.com/aetheris-dev/aetheris/server/internal/authorization"
	"github.com/aetheris-dev/aetheris/server/internal/httpx"
)

func deviceProjectIdentityKey(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	principal, ok := devicePrincipal(w, r, deps)
	if !ok {
		return
	}
	serveDeviceProjectIdentityKey(w, r, principal, deps)
}

func serveDeviceProjectIdentityKey(w http.ResponseWriter, r *http.Request, principal authorization.Principal, deps Dependencies) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed", "message": "请求方法不受支持"})
		return
	}
	if principal.Kind != "device" || authorization.Require(principal, "projects:register", authorization.Scope{TenantID: principal.TenantID}) != nil {
		httpx.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "message": "设备没有项目注册权限"})
		return
	}
	if len(deps.ProjectIdentityKey) < 32 || deps.ProjectIdentityKeyVersion < 1 {
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "project_identity_key_unavailable", "message": "项目身份服务尚未正确配置"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"key":     base64.StdEncoding.EncodeToString(deps.ProjectIdentityKey),
		"version": deps.ProjectIdentityKeyVersion,
	})
}
