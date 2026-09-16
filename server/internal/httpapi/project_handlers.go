package httpapi

import (
	"net/http"

	"github.com/aetheris-dev/aetheris/server/internal/authorization"
	"github.com/aetheris-dev/aetheris/server/internal/httpx"
	"github.com/aetheris-dev/aetheris/server/internal/projects"
)

func deviceProjects(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	principal, ok := devicePrincipal(w, r, deps)
	if !ok {
		return
	}
	serveDeviceProjects(w, r, principal, deps)
}

func serveDeviceProjects(w http.ResponseWriter, r *http.Request, principal authorization.Principal, deps Dependencies) {
	if r.Method != http.MethodPut {
		httpx.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed", "message": "请求方法不受支持"})
		return
	}
	if principal.Kind != "device" || authorization.Require(principal, "projects:register", authorization.Scope{TenantID: principal.TenantID}) != nil {
		httpx.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "message": "设备没有项目注册权限"})
		return
	}
	if deps.Projects == nil {
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "project_registry_unavailable", "message": "项目注册服务暂不可用"})
		return
	}
	var batch projects.RegistrationBatch
	if err := httpx.ReadJSON(r, 256*1024, &batch); err != nil || len(batch.Projects) == 0 || len(batch.Projects) > 100 {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_project_batch", "message": "项目注册批次格式无效或数量超限"})
		return
	}
	for _, registration := range batch.Projects {
		if err := projects.ValidateRegistration(registration); err != nil {
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_project_registration", "message": err.Error()})
			return
		}
	}
	result, err := deps.Projects.Register(r.Context(), principal, batch)
	if err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "project_registration_failed", "message": "项目注册失败"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func adminProjects(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	withSession(w, r, deps, func(w http.ResponseWriter, r *http.Request, principal authorization.Principal) {
		serveAdminProjects(w, r, principal, deps)
	})
}

func serveAdminProjects(w http.ResponseWriter, r *http.Request, principal authorization.Principal, deps Dependencies) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed", "message": "请求方法不受支持"})
		return
	}
	if authorization.Require(principal, "projects:manage", authorization.Scope{TenantID: principal.TenantID}) != nil {
		httpx.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "message": "没有项目管理权限"})
		return
	}
	if deps.Projects == nil {
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "project_registry_unavailable", "message": "项目注册服务暂不可用"})
		return
	}
	items, err := deps.Projects.List(r.Context(), principal.TenantID)
	if err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "project_query_failed", "message": "项目列表加载失败"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"projects": items, "count": len(items)})
}
