package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/aetheris-dev/aetheris/server/internal/authorization"
	"github.com/aetheris-dev/aetheris/server/internal/effectiveness"
	"github.com/aetheris-dev/aetheris/server/internal/httpx"
)

func adminEffectivenessReport(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	withSession(w, r, deps, func(w http.ResponseWriter, r *http.Request, principal authorization.Principal) {
		serveEffectivenessReport(w, r, principal, deps)
	})
}

func serveEffectivenessReport(w http.ResponseWriter, r *http.Request, principal authorization.Principal, deps Dependencies) {
	if principal.Kind != "user" {
		httpx.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "message": "设备身份不能读取个人效能"})
		return
	}
	subjectID := r.URL.Query().Get("subject_id")
	if subjectID == "" {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "subject_required", "message": "必须选择主体"})
		return
	}
	location := time.UTC
	if deps.Effectiveness != nil {
		location = deps.Effectiveness.Location()
	}
	from, to, err := parseEffectivenessRange(r, location)
	if err != nil || effectiveness.ValidateQueryRange(from, to) != nil {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_range", "message": "日期范围必须为 1 到 90 天"})
		return
	}
	if err := authorization.Require(principal, "effectiveness:read", authorization.Scope{TenantID: principal.TenantID}); err != nil {
		httpx.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	if deps.Effectiveness == nil {
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "effectiveness_unavailable"})
		return
	}
	report, err := deps.Effectiveness.Report(r.Context(), principal, subjectID, from, to)
	if err != nil {
		if strings.Contains(err.Error(), "本人") {
			httpx.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "message": err.Error()})
		} else {
			httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "effectiveness_query_failed"})
		}
		return
	}
	httpx.WriteJSON(w, http.StatusOK, report)
}

func adminEffectivenessSubjects(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	withSession(w, r, deps, func(w http.ResponseWriter, r *http.Request, principal authorization.Principal) {
		if principal.Kind != "user" || authorization.Require(principal, "effectiveness:read", authorization.Scope{TenantID: principal.TenantID}) != nil {
			httpx.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		if deps.Effectiveness == nil {
			httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "effectiveness_unavailable"})
			return
		}
		from, to, err := parseEffectivenessRange(r, deps.Effectiveness.Location())
		if err != nil || effectiveness.ValidateQueryRange(from, to) != nil {
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_range"})
			return
		}
		items, err := deps.Effectiveness.ListSubjects(r.Context(), principal, from, to)
		if err != nil {
			httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "effectiveness_query_failed"})
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"subjects": items, "count": len(items)})
	})
}

func adminEffectivenessRecompute(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	withSession(w, r, deps, func(w http.ResponseWriter, r *http.Request, principal authorization.Principal) {
		if r.Method != http.MethodPost {
			httpx.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
			return
		}
		if principal.Kind != "user" || authorization.Require(principal, "effectiveness:manage", authorization.Scope{TenantID: principal.TenantID}) != nil {
			httpx.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		if deps.Effectiveness == nil {
			httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "effectiveness_unavailable"})
			return
		}
		var body struct {
			From string `json:"from"`
			To   string `json:"to"`
		}
		if err := httpx.ReadJSON(r, 64*1024, &body); err != nil {
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
			return
		}
		from, err1 := time.ParseInLocation("2006-01-02", body.From, deps.Effectiveness.Location())
		to, err2 := time.ParseInLocation("2006-01-02", body.To, deps.Effectiveness.Location())
		if err1 != nil || err2 != nil || effectiveness.ValidateRecomputeRange(from, to) != nil {
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_range"})
			return
		}
		jobID, err := deps.Effectiveness.RecomputeRange(r.Context(), principal.TenantID, from, to)
		if err != nil {
			httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "recompute_failed"})
			return
		}
		httpx.WriteJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID, "status": "queued"})
	})
}

func adminEffectivenessSummary(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	withSession(w, r, deps, func(w http.ResponseWriter, r *http.Request, principal authorization.Principal) {
		if r.Method != http.MethodPost {
			httpx.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
			return
		}
		if authorization.Require(principal, "models:invoke", authorization.Scope{TenantID: principal.TenantID}) != nil {
			httpx.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		if deps.Effectiveness == nil {
			httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "effectiveness_unavailable"})
			return
		}
		var body struct {
			SubjectID string `json:"subject_id"`
			From      string `json:"from"`
			To        string `json:"to"`
		}
		if err := httpx.ReadJSON(r, 64*1024, &body); err != nil || body.SubjectID == "" {
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
			return
		}
		from, err1 := time.ParseInLocation("2006-01-02", body.From, deps.Effectiveness.Location())
		to, err2 := time.ParseInLocation("2006-01-02", body.To, deps.Effectiveness.Location())
		if err1 != nil || err2 != nil || effectiveness.ValidateQueryRange(from, to) != nil {
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_range"})
			return
		}
		report, err := deps.Effectiveness.Report(r.Context(), principal, body.SubjectID, from, to)
		if err != nil {
			httpx.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		if deps.ModelGatewayURL == "" {
			httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "model_gateway_disabled"})
			return
		}
		payload, _ := json.Marshal(map[string]any{
			"tenant_id": principal.TenantID, "actor_id": principal.ID, "task": "summarize_personal_effectiveness",
			"model": "default", "input_event_ids": report.EvidenceEventIDs,
			"context":  map[string]any{"effectiveness": effectivenessSummaryContext(report), "notice": "AI 总结，不作为绩效评价"},
			"messages": []map[string]string{{"role": "system", "content": "只描述可解释指标和数据覆盖度，不生成排名或综合分数。"}},
		})
		request, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, strings.TrimRight(deps.ModelGatewayURL, "/")+"/internal/v1/generate", bytes.NewReader(payload))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+deps.ModelGatewayToken)
		response, err := modelGatewayClient(deps.ModelGatewayTimeout).Do(request)
		if err != nil {
			httpx.WriteJSON(w, http.StatusBadGateway, map[string]string{"error": "model_gateway_unavailable"})
			return
		}
		defer response.Body.Close()
		var result map[string]any
		if json.NewDecoder(response.Body).Decode(&result) != nil || response.StatusCode >= 400 {
			httpx.WriteJSON(w, http.StatusBadGateway, map[string]string{"error": "model_gateway_error"})
			return
		}
		result["notice"] = "AI 总结，不作为绩效评价"
		httpx.WriteJSON(w, http.StatusOK, result)
	})
}

func effectivenessSummaryContext(report effectiveness.Report) map[string]any {
	return map[string]any{
		"subject_id":                report.SubjectID,
		"from":                      report.From,
		"to":                        report.To,
		"timezone":                  report.Timezone,
		"metric_definition_version": report.MetricDefinitionVersion,
		"totals":                    report.Totals,
		"project_breakdown":         report.ProjectBreakdown,
		"work_role_breakdown":       report.WorkRoleBreakdown,
		"activity_breakdown":        report.ActivityBreakdown,
		"trends":                    report.Trends,
		"coverage":                  report.Coverage,
		"definitions":               report.Definitions,
	}
}

func parseEffectivenessRange(r *http.Request, location *time.Location) (time.Time, time.Time, error) {
	now := time.Now().In(location)
	to := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
	from := to.AddDate(0, 0, -29)
	var err error
	if raw := r.URL.Query().Get("from"); raw != "" {
		from, err = time.ParseInLocation("2006-01-02", raw, location)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
	}
	if raw := r.URL.Query().Get("to"); raw != "" {
		to, err = time.ParseInLocation("2006-01-02", raw, location)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
	}
	return from, to, nil
}
