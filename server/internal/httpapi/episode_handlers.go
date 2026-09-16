package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/aetheris-dev/aetheris/server/internal/authorization"
	"github.com/aetheris-dev/aetheris/server/internal/episodes"
	"github.com/aetheris-dev/aetheris/server/internal/httpx"
)

func adminEpisodes(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	withSession(w, r, deps, func(w http.ResponseWriter, r *http.Request, principal authorization.Principal) {
		serveAdminEpisodes(w, r, principal, deps)
	})
}

func serveAdminEpisodes(w http.ResponseWriter, r *http.Request, principal authorization.Principal, deps Dependencies) {
	if authorization.Require(principal, "events:read", authorization.Scope{TenantID: principal.TenantID}) != nil {
		httpx.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "message": "没有工作片段查看权限"})
		return
	}
	if deps.Episodes == nil || r.Method != http.MethodGet {
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "episodes_unavailable", "message": "工作片段服务暂不可用"})
		return
	}
	const prefix = "/api/v1/admin/work-episodes"
	relative := strings.Trim(strings.TrimPrefix(r.URL.Path, prefix), "/")
	if relative != "" {
		item, err := deps.Episodes.Get(r.Context(), principal.TenantID, relative)
		if err != nil {
			httpx.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "episode_not_found", "message": "工作片段不存在"})
			return
		}
		httpx.WriteJSON(w, http.StatusOK, item)
		return
	}
	limit, offset := parsePage(r.URL.Query().Get("limit"), r.URL.Query().Get("offset"))
	items, err := deps.Episodes.List(r.Context(), principal.TenantID, episodes.Filter{
		ProjectID: r.URL.Query().Get("project_id"), SubjectID: r.URL.Query().Get("subject_id"), Status: r.URL.Query().Get("status"), Limit: limit, Offset: offset,
		DeviceID: r.URL.Query().Get("device_id"),
	})
	if err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "episodes_query_failed", "message": "工作片段加载失败"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"episodes": items, "count": len(items), "limit": limit, "offset": offset})
}

func parsePage(rawLimit, rawOffset string) (int, int) {
	limit, _ := strconv.Atoi(rawLimit)
	if limit < 1 || limit > 100 {
		limit = 50
	}
	offset, _ := strconv.Atoi(rawOffset)
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
