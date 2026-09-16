package httpapi

import (
	"net/http"
	"time"

	"github.com/aetheris-dev/aetheris/server/internal/authorization"
	"github.com/aetheris-dev/aetheris/server/internal/cleaning"
	"github.com/aetheris-dev/aetheris/server/internal/httpx"
)

func adminDataQualitySummary(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	withSession(w, r, deps, func(w http.ResponseWriter, r *http.Request, principal authorization.Principal) {
		if authorization.Require(principal, "events:read", authorization.Scope{TenantID: principal.TenantID}) != nil {
			httpx.WriteJSON(w, 403, map[string]string{"error": "forbidden"})
			return
		}
		from, to, ok := dataQualityRange(w, r, deps)
		if !ok {
			return
		}
		result, err := deps.Cleaning.Summary(r.Context(), principal.TenantID, from, to)
		if err != nil {
			httpx.WriteJSON(w, 500, map[string]string{"error": "data_quality_query_failed", "message": "数据质量汇总加载失败"})
			return
		}
		httpx.WriteJSON(w, 200, result)
	})
}

func adminDataQualityFacts(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	withSession(w, r, deps, func(w http.ResponseWriter, r *http.Request, principal authorization.Principal) {
		if authorization.Require(principal, "events:read", authorization.Scope{TenantID: principal.TenantID}) != nil {
			httpx.WriteJSON(w, 403, map[string]string{"error": "forbidden"})
			return
		}
		from, to, ok := dataQualityRange(w, r, deps)
		if !ok {
			return
		}
		limit, offset := queryInt(r, "limit", 50), queryInt(r, "offset", 0)
		if limit < 1 || limit > 100 {
			limit = 50
		}
		if offset < 0 {
			offset = 0
		}
		quality := r.URL.Query().Get("quality_state")
		if quality != "" && quality != "accepted" && quality != "merged" && quality != "quarantined" {
			httpx.WriteJSON(w, 400, map[string]string{"error": "invalid_quality_state"})
			return
		}
		result, err := deps.Cleaning.ListFacts(r.Context(), principal.TenantID, from, to, limit, offset, quality)
		if err != nil {
			httpx.WriteJSON(w, 500, map[string]string{"error": "data_quality_query_failed", "message": "清洗事实加载失败"})
			return
		}
		httpx.WriteJSON(w, 200, result)
	})
}

func adminDataQualityRecompute(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	withSession(w, r, deps, func(w http.ResponseWriter, r *http.Request, principal authorization.Principal) {
		if deps.Cleaning == nil {
			httpx.WriteJSON(w, 503, map[string]string{"error": "cleaning_unavailable"})
			return
		}
		if r.Method != http.MethodPost {
			httpx.WriteJSON(w, 405, map[string]string{"error": "method_not_allowed"})
			return
		}
		if authorization.Require(principal, "cleaning:manage", authorization.Scope{TenantID: principal.TenantID}) != nil {
			httpx.WriteJSON(w, 403, map[string]string{"error": "forbidden"})
			return
		}
		var body struct {
			From string `json:"from"`
			To   string `json:"to"`
		}
		if err := httpx.ReadJSON(r, 64*1024, &body); err != nil {
			httpx.WriteJSON(w, 400, map[string]string{"error": "invalid_request"})
			return
		}
		location := deps.Cleaning.Location()
		from, err1 := time.ParseInLocation("2006-01-02", body.From, location)
		to, err2 := time.ParseInLocation("2006-01-02", body.To, location)
		if err1 != nil || err2 != nil || cleaning.ValidateRange(from, to) != nil {
			httpx.WriteJSON(w, 400, map[string]string{"error": "invalid_range", "message": "日期范围必须为 1 到 90 天"})
			return
		}
		jobID, err := deps.Cleaning.RecomputeRange(r.Context(), principal.TenantID, from, to, cleaning.CurrentRuleVersion)
		if err != nil {
			httpx.WriteJSON(w, 500, map[string]string{"error": "cleaning_recompute_failed"})
			return
		}
		httpx.WriteJSON(w, 202, map[string]string{"job_id": jobID, "status": "queued"})
	})
}

func dataQualityRange(w http.ResponseWriter, r *http.Request, deps Dependencies) (time.Time, time.Time, bool) {
	if deps.Cleaning == nil {
		httpx.WriteJSON(w, 503, map[string]string{"error": "cleaning_unavailable"})
		return time.Time{}, time.Time{}, false
	}
	location := deps.Cleaning.Location()
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
	if err != nil || cleaning.ValidateRange(from, to) != nil {
		httpx.WriteJSON(w, 400, map[string]string{"error": "invalid_range", "message": "日期范围必须为 1 到 90 天"})
		return time.Time{}, time.Time{}, false
	}
	return from, to, true
}
