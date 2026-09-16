package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/aetheris-dev/aetheris/server/internal/aiinteractions"
	"github.com/aetheris-dev/aetheris/server/internal/authorization"
	"github.com/aetheris-dev/aetheris/server/internal/httpx"
)

func adminAIInteractions(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	withSession(w, r, deps, func(w http.ResponseWriter, r *http.Request, principal authorization.Principal) {
		if authorization.Require(principal, "events:read", authorization.Scope{TenantID: principal.TenantID}) != nil {
			httpx.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		if deps.AIInteractions == nil {
			httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ai_interactions_unavailable", "message": "AI 交互查询暂不可用"})
			return
		}
		location := time.UTC
		if deps.Effectiveness != nil {
			location = deps.Effectiveness.Location()
		}
		now := time.Now().In(location)
		to := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
		from := to.AddDate(0, 0, -29)
		var err error
		if value := r.URL.Query().Get("from"); value != "" {
			from, err = time.ParseInLocation("2006-01-02", value, location)
		}
		if err == nil {
			if value := r.URL.Query().Get("to"); value != "" {
				to, err = time.ParseInLocation("2006-01-02", value, location)
			}
		}
		if err != nil || to.Before(from) || int(to.Sub(from).Hours()/24)+1 > 90 {
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_range", "message": "日期范围必须为 1 到 90 天"})
			return
		}
		role := r.URL.Query().Get("message_role")
		if role != "" && role != "user" && role != "assistant" && role != "ai_tool" && role != "system" && role != "tool" && role != "unknown" {
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_message_role", "message": "消息角色无效"})
			return
		}
		limit := queryInt(r, "limit", 50)
		if limit < 1 || limit > 100 {
			limit = 50
		}
		offset := queryInt(r, "offset", 0)
		if offset < 0 {
			offset = 0
		}
		result, err := deps.AIInteractions.Query(r.Context(), aiinteractions.Filter{
			TenantID: principal.TenantID, DeviceID: r.URL.Query().Get("device_id"), ProjectID: r.URL.Query().Get("project_id"),
			MessageRole: role, From: from, ToExclusive: to.AddDate(0, 0, 1), Limit: limit, Offset: offset,
		})
		if err != nil {
			httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "ai_interactions_query_failed", "message": "AI 交互数据加载失败"})
			return
		}
		httpx.WriteJSON(w, http.StatusOK, result)
	})
}

func queryInt(r *http.Request, name string, fallback int) int {
	value, err := strconv.Atoi(r.URL.Query().Get(name))
	if err != nil {
		return fallback
	}
	return value
}
