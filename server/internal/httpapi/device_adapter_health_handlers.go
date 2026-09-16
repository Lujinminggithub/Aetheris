package httpapi

import (
	"net/http"
	"strings"

	"github.com/aetheris-dev/aetheris/server/internal/adapterhealth"
	"github.com/aetheris-dev/aetheris/server/internal/authorization"
	"github.com/aetheris-dev/aetheris/server/internal/httpx"
)

func deviceAdapterHealth(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	principal, ok := devicePrincipal(w, r, deps)
	if !ok {
		return
	}
	serveDeviceAdapterHealth(w, r, principal, deps)
}

func serveDeviceAdapterHealth(w http.ResponseWriter, r *http.Request, principal authorization.Principal, deps Dependencies) {
	if r.Method != http.MethodPost || principal.Kind != "device" || authorization.Require(principal, "health:report", authorization.Scope{TenantID: principal.TenantID}) != nil {
		httpx.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "message": "设备没有采集健康上报权限"})
		return
	}
	if deps.AdapterHealth == nil {
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "adapter_health_unavailable", "message": "采集健康服务暂不可用"})
		return
	}
	var body struct {
		Snapshots []adapterhealth.Snapshot `json:"snapshots"`
	}
	if err := httpx.ReadJSON(r, 256*1024, &body); err != nil || len(body.Snapshots) == 0 || len(body.Snapshots) > 100 {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_adapter_health", "message": "采集健康快照格式无效"})
		return
	}
	for _, snapshot := range body.Snapshots {
		if snapshot.DeviceID != "" && snapshot.DeviceID != principal.DeviceID {
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "adapter_health_identity_mismatch", "message": "采集健康快照设备身份不匹配"})
			return
		}
		snapshot.TenantID = principal.TenantID
		snapshot.DeviceID = principal.DeviceID
		if snapshot.AdapterID == "" || strings.ContainsAny(snapshot.AdapterID, "\\/\n\r") {
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_adapter_health", "message": "适配器标识无效"})
			return
		}
		if !adapterhealth.ValidComponentState(snapshot.ComponentState) || snapshot.ProtocolVersion < 0 || snapshot.PendingEvents < 0 || snapshot.SentEvents < 0 || snapshot.DroppedEvents < 0 || len(snapshot.ComponentVersion) > 128 || len(snapshot.VSCodeVersion) > 128 {
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_component_state", "message": "采集组件状态格式无效"})
			return
		}
		if err := deps.AdapterHealth.Upsert(r.Context(), snapshot); err != nil {
			httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "adapter_health_save_failed", "message": "采集健康保存失败"})
			return
		}
	}
	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"accepted": len(body.Snapshots)})
}
