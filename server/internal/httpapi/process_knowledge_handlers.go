package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/aetheris-dev/aetheris/server/internal/authorization"
	"github.com/aetheris-dev/aetheris/server/internal/httpx"
	"github.com/aetheris-dev/aetheris/server/internal/processknowledge"
)

type ProcessKnowledgeService interface {
	Summary(context.Context, string) (processknowledge.Summary, error)
	ListUnits(context.Context, processknowledge.ListFilter) ([]processknowledge.KnowledgeUnit, int, error)
	GetUnit(context.Context, string, string) (processknowledge.KnowledgeUnit, error)
	StartBackfill(context.Context, string, string, string, string, int) (processknowledge.Job, error)
	GetJob(context.Context, string, string) (processknowledge.Job, error)
	Activate(context.Context, string, int, string, int) error
	Rollback(context.Context, string, int) error
}

func adminProcessKnowledge(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	withSession(w, r, deps, func(w http.ResponseWriter, r *http.Request, principal authorization.Principal) {
		serveAdminProcessKnowledge(w, r, principal, deps)
	})
}

func serveAdminProcessKnowledge(w http.ResponseWriter, r *http.Request, principal authorization.Principal, deps Dependencies) {
	if deps.ProcessKnowledge == nil {
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "process_knowledge_unavailable"})
		return
	}
	relative := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/process-knowledge/"), "/")
	manage := relative == "backfills" || strings.HasPrefix(relative, "jobs/") || strings.HasPrefix(relative, "versions/")
	permission := "process_knowledge:read"
	if manage {
		permission = "process_knowledge:manage"
	}
	if authorization.Require(principal, permission, authorization.Scope{TenantID: principal.TenantID}) != nil {
		httpx.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	switch {
	case relative == "summary" && r.Method == http.MethodGet:
		result, err := deps.ProcessKnowledge.Summary(r.Context(), principal.TenantID)
		if err != nil {
			httpx.WriteJSON(w, 500, map[string]string{"error": "process_knowledge_query_failed"})
			return
		}
		httpx.WriteJSON(w, 200, result)
	case relative == "units" && r.Method == http.MethodGet:
		filter := processknowledge.ListFilter{TenantID: principal.TenantID, LogicalProjectID: r.URL.Query().Get("logical_project_id"), Topic: r.URL.Query().Get("topic"), ValidationState: r.URL.Query().Get("validation_state"), DecisionState: r.URL.Query().Get("decision_state"), Limit: queryInt(r, "limit", 50), Offset: queryInt(r, "offset", 0)}
		items, total, err := deps.ProcessKnowledge.ListUnits(r.Context(), filter)
		if err != nil {
			httpx.WriteJSON(w, 500, map[string]string{"error": "process_knowledge_query_failed"})
			return
		}
		httpx.WriteJSON(w, 200, map[string]any{"units": items, "total": total, "limit": filter.Limit, "offset": filter.Offset})
	case strings.HasPrefix(relative, "units/") && r.Method == http.MethodGet:
		id := strings.TrimPrefix(relative, "units/")
		item, err := deps.ProcessKnowledge.GetUnit(r.Context(), principal.TenantID, id)
		if err != nil {
			httpx.WriteJSON(w, 404, map[string]string{"error": "process_knowledge_not_found"})
			return
		}
		httpx.WriteJSON(w, 200, item)
	case relative == "backfills" && r.Method == http.MethodPost:
		var body struct {
			Mode             string `json:"mode"`
			LogicalProjectID string `json:"logical_project_id"`
			Version          int    `json:"version"`
		}
		if err := httpx.ReadJSON(r, 64*1024, &body); err != nil {
			httpx.WriteJSON(w, 400, map[string]string{"error": "invalid_request"})
			return
		}
		job, err := deps.ProcessKnowledge.StartBackfill(r.Context(), principal.TenantID, principal.ID, body.Mode, body.LogicalProjectID, body.Version)
		if err != nil {
			httpx.WriteJSON(w, 400, map[string]string{"error": "process_knowledge_backfill_failed", "message": err.Error()})
			return
		}
		httpx.WriteJSON(w, 202, job)
	case strings.HasPrefix(relative, "jobs/") && r.Method == http.MethodGet:
		job, err := deps.ProcessKnowledge.GetJob(r.Context(), principal.TenantID, strings.TrimPrefix(relative, "jobs/"))
		if err != nil {
			httpx.WriteJSON(w, 404, map[string]string{"error": "process_knowledge_job_not_found"})
			return
		}
		httpx.WriteJSON(w, 200, job)
	case strings.HasPrefix(relative, "versions/") && r.Method == http.MethodPost:
		parts := strings.Split(relative, "/")
		if len(parts) != 3 {
			httpx.WriteJSON(w, 404, map[string]string{"error": "not_found"})
			return
		}
		version, err := strconv.Atoi(parts[1])
		if err != nil {
			httpx.WriteJSON(w, 400, map[string]string{"error": "invalid_version"})
			return
		}
		if parts[2] == "rollback" {
			err = deps.ProcessKnowledge.Rollback(r.Context(), principal.TenantID, version)
		} else if parts[2] == "activate" {
			var body struct {
				Mode          string `json:"mode"`
				CanaryPercent int    `json:"canary_percent"`
			}
			if readErr := httpx.ReadJSON(r, 64*1024, &body); readErr != nil {
				httpx.WriteJSON(w, 400, map[string]string{"error": "invalid_request"})
				return
			}
			err = deps.ProcessKnowledge.Activate(r.Context(), principal.TenantID, version, body.Mode, body.CanaryPercent)
		} else {
			httpx.WriteJSON(w, 404, map[string]string{"error": "not_found"})
			return
		}
		if err != nil {
			httpx.WriteJSON(w, 400, map[string]string{"error": "process_knowledge_version_failed", "message": err.Error()})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		httpx.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
	}
}
