package httpapi

import (
	"net/http"
	"strings"

	"github.com/aetheris-dev/aetheris/server/internal/authorization"
	"github.com/aetheris-dev/aetheris/server/internal/httpx"
	"github.com/aetheris-dev/aetheris/server/internal/retrieval"
)

func adminRAGStatus(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	withSession(w, r, deps, func(w http.ResponseWriter, r *http.Request, principal authorization.Principal) {
		if r.Method != http.MethodGet {
			httpx.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
			return
		}
		if authorization.Require(principal, "retrieval:query", authorization.Scope{TenantID: principal.TenantID}) != nil {
			httpx.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		if deps.Retrieval == nil {
			httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "retrieval_unavailable", "message": "智能查询暂不可用"})
			return
		}
		result, err := deps.Retrieval.Status(r.Context(), principal.TenantID)
		if err != nil {
			httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "retrieval_status_failed"})
			return
		}
		httpx.WriteJSON(w, http.StatusOK, result)
	})
}

func adminRAGQueries(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	withSession(w, r, deps, func(w http.ResponseWriter, r *http.Request, principal authorization.Principal) {
		if authorization.Require(principal, "retrieval:query", authorization.Scope{TenantID: principal.TenantID}) != nil {
			httpx.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		if deps.Retrieval == nil {
			httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "retrieval_unavailable", "message": "智能查询暂不可用"})
			return
		}
		switch r.Method {
		case http.MethodPost:
			if r.URL.Path != "/api/v1/admin/rag/queries" {
				httpx.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "query_not_found"})
				return
			}
			var input retrieval.QueryInput
			if err := httpx.ReadJSON(r, 64*1024, &input); err != nil {
				httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
				return
			}
			job, err := deps.Retrieval.Create(r.Context(), principal.TenantID, principal.ID, input)
			if err != nil {
				httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_query", "message": err.Error()})
				return
			}
			httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"query_id": job.ID, "status": job.Status, "progress": job.Progress})
		case http.MethodGet:
			id := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/rag/queries/")
			if id == "" || strings.Contains(id, "/") {
				httpx.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "query_not_found"})
				return
			}
			job, err := deps.Retrieval.Get(r.Context(), principal.TenantID, principal.ID, id)
			if err != nil {
				httpx.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "query_not_found"})
				return
			}
			httpx.WriteJSON(w, http.StatusOK, job)
		default:
			httpx.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		}
	})
}
