package httpapi

import (
	"net/http"

	"github.com/aetheris-dev/aetheris/server/internal/adapterhealth"
	"github.com/aetheris-dev/aetheris/server/internal/authorization"
	"github.com/aetheris-dev/aetheris/server/internal/httpx"
)

func adminAdapterHealth(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	withSession(w, r, deps, func(w http.ResponseWriter, r *http.Request, principal authorization.Principal) {
		if authorization.Require(principal, "events:read", authorization.Scope{TenantID: principal.TenantID}) != nil {
			httpx.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "message": "没有采集健康查看权限"})
			return
		}
		if deps.AdapterHealth == nil || r.Method != http.MethodGet {
			httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "adapter_health_unavailable", "message": "采集健康服务暂不可用"})
			return
		}
		items, err := deps.AdapterHealth.List(r.Context(), principal.TenantID, r.URL.Query().Get("device_id"))
		if err != nil {
			httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "adapter_health_query_failed", "message": "采集健康加载失败"})
			return
		}
		adapterID := r.URL.Query().Get("adapter_id")
		filtered := make([]adapterhealth.Snapshot, 0, len(items))
		for _, item := range items {
			if adapterID != "" && item.AdapterID != adapterID {
				continue
			}
			filtered = append(filtered, adapterhealth.SafeSnapshot(item))
		}
		items = filtered
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items, "count": len(items)})
	})
}
