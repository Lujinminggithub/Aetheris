package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/aetheris-dev/aetheris/server/internal/authorization"
	"github.com/aetheris-dev/aetheris/server/internal/httpx"
	"github.com/aetheris-dev/aetheris/server/internal/publicknowledge"
)

type PublicKnowledgeService interface {
	Summary(context.Context) (publicknowledge.Summary, error)
	List(context.Context, publicknowledge.ListFilter) ([]publicknowledge.Unit, int, error)
	Get(context.Context, string, string) (publicknowledge.Unit, error)
	Confirm(context.Context, publicknowledge.ReviewCommand) (publicknowledge.Unit, error)
	Verify(context.Context, publicknowledge.ReviewCommand) (publicknowledge.Unit, error)
	Certify(context.Context, publicknowledge.ReviewCommand) (publicknowledge.Unit, error)
	Reject(context.Context, publicknowledge.ReviewCommand) (publicknowledge.Unit, error)
	Suspend(context.Context, publicknowledge.ReviewCommand) (publicknowledge.Unit, error)
	Withdraw(context.Context, publicknowledge.ReviewCommand) (publicknowledge.Unit, error)
	Republish(context.Context, publicknowledge.ReviewCommand) (publicknowledge.Unit, error)
	StartBuild(context.Context, string, string) (publicknowledge.Job, error)
	GetJob(context.Context, string) (publicknowledge.Job, error)
}

func adminPublicKnowledge(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	withSession(w, r, deps, func(w http.ResponseWriter, r *http.Request, principal authorization.Principal) {
		serveAdminPublicKnowledge(w, r, principal, deps)
	})
}

func serveAdminPublicKnowledge(w http.ResponseWriter, r *http.Request, principal authorization.Principal, deps Dependencies) {
	if deps.PublicKnowledge == nil {
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "public_knowledge_unavailable"})
		return
	}
	relative := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/public-knowledge/"), "/")
	permission := publicKnowledgePermission(relative, r.Method)
	if permission == "" {
		httpx.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if authorization.Require(principal, permission, authorization.Scope{TenantID: principal.TenantID}) != nil {
		httpx.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	switch {
	case relative == "summary" && r.Method == http.MethodGet:
		result, err := deps.PublicKnowledge.Summary(r.Context())
		writePublicKnowledgeResult(w, result, err)
	case relative == "units" && r.Method == http.MethodGet:
		filter := publicknowledge.ListFilter{
			PublicationState: r.URL.Query().Get("publication_state"), ValidationState: r.URL.Query().Get("validation_state"),
			Topic: r.URL.Query().Get("topic"), ViewerTenantID: principal.TenantID,
			Limit: queryInt(r, "limit", 50), Offset: queryInt(r, "offset", 0),
		}
		items, total, err := deps.PublicKnowledge.List(r.Context(), filter)
		if err != nil {
			writePublicKnowledgeError(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"units": items, "total": total, "limit": filter.Limit, "offset": filter.Offset})
	case strings.HasPrefix(relative, "units/") && r.Method == http.MethodGet:
		parts := strings.Split(relative, "/")
		if len(parts) != 2 {
			httpx.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "public_knowledge_not_found"})
			return
		}
		item, err := deps.PublicKnowledge.Get(r.Context(), parts[1], principal.TenantID)
		writePublicKnowledgeResult(w, item, err)
	case strings.HasPrefix(relative, "units/") && r.Method == http.MethodPost:
		servePublicKnowledgeReview(w, r, principal, deps, relative)
	case relative == "jobs" && r.Method == http.MethodPost:
		var body struct {
			Mode string `json:"mode"`
		}
		if err := httpx.ReadJSON(r, 64*1024, &body); err != nil {
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
			return
		}
		job, err := deps.PublicKnowledge.StartBuild(r.Context(), principal.ID, body.Mode)
		if err != nil {
			writePublicKnowledgeError(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusAccepted, job)
	case strings.HasPrefix(relative, "jobs/") && r.Method == http.MethodGet:
		job, err := deps.PublicKnowledge.GetJob(r.Context(), strings.TrimPrefix(relative, "jobs/"))
		writePublicKnowledgeResult(w, job, err)
	default:
		httpx.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
	}
}

func publicKnowledgePermission(relative, method string) string {
	if method == http.MethodGet {
		return "knowledge:diagnose"
	}
	if method != http.MethodPost {
		return ""
	}
	if relative == "jobs" {
		return "knowledge:certify_public"
	}
	action := ""
	if parts := strings.Split(relative, "/"); len(parts) == 3 && parts[0] == "units" {
		action = parts[2]
	}
	switch action {
	case "confirm":
		return "knowledge:confirm_source"
	case "verify":
		return "knowledge:verify"
	case "certify", "reject":
		return "knowledge:certify_public"
	case "suspend", "withdraw", "republish":
		return "knowledge:withdraw_public"
	default:
		return ""
	}
}

func servePublicKnowledgeReview(w http.ResponseWriter, r *http.Request, principal authorization.Principal, deps Dependencies, relative string) {
	parts := strings.Split(relative, "/")
	if len(parts) != 3 || parts[0] != "units" || parts[1] == "" {
		httpx.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "public_knowledge_not_found"})
		return
	}
	var body struct {
		ExpectedRevision int    `json:"expected_revision"`
		Reason           string `json:"reason"`
	}
	if err := httpx.ReadJSON(r, 64*1024, &body); err != nil || body.ExpectedRevision < 1 || strings.TrimSpace(body.Reason) == "" {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	command := publicknowledge.ReviewCommand{KnowledgeID: parts[1], ExpectedRevision: body.ExpectedRevision, ActorID: principal.ID, ActorTenantID: principal.TenantID, Reason: body.Reason}
	var item publicknowledge.Unit
	var err error
	switch parts[2] {
	case "confirm":
		item, err = deps.PublicKnowledge.Confirm(r.Context(), command)
	case "verify":
		item, err = deps.PublicKnowledge.Verify(r.Context(), command)
	case "certify":
		item, err = deps.PublicKnowledge.Certify(r.Context(), command)
	case "reject":
		item, err = deps.PublicKnowledge.Reject(r.Context(), command)
	case "suspend":
		item, err = deps.PublicKnowledge.Suspend(r.Context(), command)
	case "withdraw":
		item, err = deps.PublicKnowledge.Withdraw(r.Context(), command)
	case "republish":
		item, err = deps.PublicKnowledge.Republish(r.Context(), command)
	default:
		httpx.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "public_knowledge_action_not_found"})
		return
	}
	writePublicKnowledgeResult(w, item, err)
}

func writePublicKnowledgeResult(w http.ResponseWriter, value any, err error) {
	if err != nil {
		writePublicKnowledgeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, value)
}

func writePublicKnowledgeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, publicknowledge.ErrRevisionConflict):
		httpx.WriteJSON(w, http.StatusConflict, map[string]string{"error": "public_knowledge_revision_conflict"})
	case errors.Is(err, publicknowledge.ErrNotFound):
		httpx.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "public_knowledge_not_found"})
	case errors.Is(err, publicknowledge.ErrInvalidTransition), errors.Is(err, publicknowledge.ErrCandidateIneligible):
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "public_knowledge_invalid_state"})
	default:
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "public_knowledge_failed"})
	}
}
