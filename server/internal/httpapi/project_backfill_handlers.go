package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/aetheris-dev/aetheris/server/internal/authorization"
	"github.com/aetheris-dev/aetheris/server/internal/httpx"
	"github.com/aetheris-dev/aetheris/server/internal/projectattribution"
)

type ProjectBackfillService interface {
	Start(context.Context, string, string, string, int) (projectattribution.Job, error)
	Get(context.Context, string, string) (projectattribution.Job, error)
	Activate(context.Context, string, string) error
	Rollback(context.Context, string, int) error
}

func adminProjectBackfills(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	withSession(w, r, deps, func(w http.ResponseWriter, r *http.Request, principal authorization.Principal) {
		serveAdminProjectBackfills(w, r, principal, deps)
	})
}

func serveAdminProjectBackfills(w http.ResponseWriter, r *http.Request, principal authorization.Principal, deps Dependencies) {
	if authorization.Require(principal, "projects:manage", authorization.Scope{TenantID: principal.TenantID}) != nil {
		httpx.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "message": "没有项目管理权限"})
		return
	}
	if deps.ProjectBackfills == nil {
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "project_backfill_unavailable", "message": "历史归属服务暂不可用"})
		return
	}
	const prefix = "/api/v1/admin/project-backfills"
	relative := strings.Trim(strings.TrimPrefix(r.URL.Path, prefix), "/")
	if relative == "" {
		createProjectBackfill(w, r, principal, deps)
		return
	}
	if relative == "rollback" {
		rollbackProjectBackfill(w, r, principal, deps)
		return
	}
	parts := strings.Split(relative, "/")
	jobID := parts[0]
	if len(parts) == 2 && parts[1] == "activate" {
		if r.Method != http.MethodPost || deps.ProjectBackfills.Activate(r.Context(), principal.TenantID, jobID) != nil {
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "project_backfill_activation_failed", "message": "只有已完成的正式回填任务可以启用"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if len(parts) != 1 || r.Method != http.MethodGet {
		httpx.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed", "message": "请求方法不受支持"})
		return
	}
	job, err := deps.ProjectBackfills.Get(r.Context(), principal.TenantID, jobID)
	if err != nil {
		httpx.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "project_backfill_not_found", "message": "历史归属任务不存在"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, job)
}

func createProjectBackfill(w http.ResponseWriter, r *http.Request, principal authorization.Principal, deps Dependencies) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed", "message": "请求方法不受支持"})
		return
	}
	var body struct {
		Mode        string `json:"mode"`
		RuleVersion int    `json:"rule_version"`
	}
	if err := httpx.ReadJSON(r, 64*1024, &body); err != nil {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request", "message": "历史归属任务参数无效"})
		return
	}
	job, err := deps.ProjectBackfills.Start(r.Context(), principal.TenantID, principal.ID, body.Mode, body.RuleVersion)
	if err != nil {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "project_backfill_start_failed", "message": err.Error()})
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, job)
}

func rollbackProjectBackfill(w http.ResponseWriter, r *http.Request, principal authorization.Principal, deps Dependencies) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed", "message": "请求方法不受支持"})
		return
	}
	var body map[string]any
	if err := httpx.ReadJSON(r, 64*1024, &body); err != nil {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request", "message": "回滚参数无效"})
		return
	}
	ruleVersion, err := numericVersion(body["rule_version"])
	if err != nil || deps.ProjectBackfills.Rollback(r.Context(), principal.TenantID, ruleVersion) != nil {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "project_backfill_rollback_failed", "message": "历史归属版本回滚失败"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func numericVersion(value any) (int, error) {
	switch current := value.(type) {
	case float64:
		if current == float64(int(current)) {
			return int(current), nil
		}
	case string:
		return strconv.Atoi(current)
	}
	return 0, strconv.ErrSyntax
}
