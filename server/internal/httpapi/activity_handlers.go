package httpapi

import (
	"net/http"
	"time"

	"github.com/aetheris-dev/aetheris/server/internal/activities"
	"github.com/aetheris-dev/aetheris/server/internal/authorization"
	"github.com/aetheris-dev/aetheris/server/internal/httpx"
)

func adminActivities(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	withSession(w, r, deps, func(w http.ResponseWriter, r *http.Request, principal authorization.Principal) {
		if r.Method != http.MethodGet {
			httpx.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
			return
		}
		if authorization.Require(principal, "events:read", authorization.Scope{TenantID: principal.TenantID}) != nil {
			httpx.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		if deps.Activities == nil {
			httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "activities_unavailable"})
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
		if raw := r.URL.Query().Get("from"); raw != "" {
			from, err = time.ParseInLocation("2006-01-02", raw, location)
		}
		if err == nil {
			if raw := r.URL.Query().Get("to"); raw != "" {
				to, err = time.ParseInLocation("2006-01-02", raw, location)
			}
		}
		if err != nil || !activities.ValidateRange(from, to) {
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_range", "message": "日期范围必须为 1 到 90 天"})
			return
		}
		activityType := r.URL.Query().Get("activity_type")
		if activityType != "" && activityType != "ai" && activityType != "terminal" && activityType != "ide" && activityType != "browser" && activityType != "version_control" && activityType != "other" {
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_activity_type"})
			return
		}
		role := r.URL.Query().Get("message_role")
		if role != "" && role != "user" && role != "assistant" && role != "tool" && role != "system" && role != "unknown" {
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_message_role"})
			return
		}
		limit, offset := queryInt(r, "limit", 50), queryInt(r, "offset", 0)
		if limit < 1 || limit > 100 {
			limit = 50
		}
		if offset < 0 {
			offset = 0
		}
		result, err := deps.Activities.Query(r.Context(), activities.Filter{
			TenantID: principal.TenantID, DeviceID: r.URL.Query().Get("device_id"), ProjectID: r.URL.Query().Get("project_id"),
			ActivityType: activityType, MessageRole: role, From: from, ToExclusive: to.AddDate(0, 0, 1), Limit: limit, Offset: offset,
		})
		if err != nil {
			httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "activities_query_failed", "message": "活动记录加载失败"})
			return
		}
		httpx.WriteJSON(w, http.StatusOK, result)
	})
}
