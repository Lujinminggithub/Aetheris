package httpapi

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aetheris-dev/aetheris/server/internal/activities"
	"github.com/aetheris-dev/aetheris/server/internal/adapterhealth"
	"github.com/aetheris-dev/aetheris/server/internal/aiinteractions"
	"github.com/aetheris-dev/aetheris/server/internal/applicationpolicy"
	"github.com/aetheris-dev/aetheris/server/internal/auth"
	"github.com/aetheris-dev/aetheris/server/internal/authorization"
	"github.com/aetheris-dev/aetheris/server/internal/browserpolicy"
	"github.com/aetheris-dev/aetheris/server/internal/cleaning"
	"github.com/aetheris-dev/aetheris/server/internal/devices"
	"github.com/aetheris-dev/aetheris/server/internal/effectiveness"
	"github.com/aetheris-dev/aetheris/server/internal/episodes"
	"github.com/aetheris-dev/aetheris/server/internal/events"
	"github.com/aetheris-dev/aetheris/server/internal/httpx"
	"github.com/aetheris-dev/aetheris/server/internal/projects"
	"github.com/aetheris-dev/aetheris/server/internal/retrieval"
	"github.com/aetheris-dev/aetheris/server/internal/workroles"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Dependencies struct {
	Auth                      *auth.Service
	Devices                   *devices.Service
	Events                    *events.Repository
	StaticDir                 string
	DatabasePing              func(context.Context) error
	Pool                      *pgxpool.Pool
	WorkRoles                 *workroles.Service
	ModelGatewayURL           string
	ModelGatewayToken         string
	ModelGatewayTimeout       time.Duration
	ClientDownloadFile        string
	Effectiveness             *effectiveness.Service
	AIInteractions            aiinteractions.Querier
	Activities                activities.Querier
	Retrieval                 *retrieval.QueryService
	Cleaning                  *cleaning.Service
	BrowserPolicies           browserpolicy.Store
	ApplicationPolicies       applicationpolicy.Store
	ProjectIdentityKey        []byte
	ProjectIdentityKeyVersion int
	Projects                  projects.Store
	ProjectBackfills          ProjectBackfillService
	AdapterHealth             adapterhealth.Store
	Episodes                  episodes.Store
}

func NewRouter(deps Dependencies) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/admin/", http.StatusFound)
	})
	mux.HandleFunc("/downloads/client", func(w http.ResponseWriter, r *http.Request) {
		serveClientDownload(w, r, deps.ClientDownloadFile)
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if deps.DatabasePing != nil {
			if err := deps.DatabasePing(r.Context()); err != nil {
				httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
				return
			}
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) { login(w, r, deps) })
	mux.HandleFunc("/api/v1/auth/logout", func(w http.ResponseWriter, r *http.Request) { logout(w, r, deps) })
	mux.HandleFunc("/api/v1/auth/refresh", func(w http.ResponseWriter, r *http.Request) { refresh(w, r, deps) })
	mux.HandleFunc("/api/v1/device/bootstrap", func(w http.ResponseWriter, r *http.Request) { bootstrap(w, r, deps) })
	mux.HandleFunc("/api/v1/device/heartbeat", func(w http.ResponseWriter, r *http.Request) { heartbeat(w, r, deps) })
	mux.HandleFunc("/api/v1/device/browser-policy", func(w http.ResponseWriter, r *http.Request) { deviceBrowserPolicy(w, r, deps) })
	mux.HandleFunc("/api/v1/device/application-capture-policy", func(w http.ResponseWriter, r *http.Request) { deviceApplicationPolicy(w, r, deps) })
	mux.HandleFunc("/api/v1/device/project-identity-key", func(w http.ResponseWriter, r *http.Request) { deviceProjectIdentityKey(w, r, deps) })
	mux.HandleFunc("/api/v1/device/projects", func(w http.ResponseWriter, r *http.Request) { deviceProjects(w, r, deps) })
	mux.HandleFunc("/api/v1/device/revoke", func(w http.ResponseWriter, r *http.Request) { revokeDevice(w, r, deps) })
	mux.HandleFunc("/api/v1/ingest", func(w http.ResponseWriter, r *http.Request) { ingest(w, r, deps) })
	// 兼容现有 Python Core 的旧版 GatewayClient 路径；业务逻辑仍复用新 ingest。
	mux.HandleFunc("/v1/events", func(w http.ResponseWriter, r *http.Request) { ingest(w, r, deps) })
	mux.HandleFunc("/v1/devices/register", func(w http.ResponseWriter, r *http.Request) { heartbeat(w, r, deps) })
	mux.HandleFunc("/api/v1/admin/me", func(w http.ResponseWriter, r *http.Request) {
		withSession(w, r, deps, func(w http.ResponseWriter, r *http.Request, p authorization.Principal) {
			httpx.WriteJSON(w, 200, map[string]any{"user_id": p.ID, "tenant_id": p.TenantID, "permissions": p.Permissions})
		})
	})
	mux.HandleFunc("/api/v1/admin/events", func(w http.ResponseWriter, r *http.Request) { adminEvents(w, r, deps) })
	mux.HandleFunc("/api/v1/admin/events/", func(w http.ResponseWriter, r *http.Request) { adminEventMutation(w, r, deps) })
	mux.HandleFunc("/api/v1/admin/summary", func(w http.ResponseWriter, r *http.Request) { adminSummary(w, r, deps) })
	mux.HandleFunc("/api/v1/admin/health/providers", func(w http.ResponseWriter, r *http.Request) { adminProviderHealth(w, r, deps) })
	mux.HandleFunc("/api/v1/admin/work-roles", func(w http.ResponseWriter, r *http.Request) { adminWorkRoles(w, r, deps) })
	mux.HandleFunc("/api/v1/admin/work-role-assignments", func(w http.ResponseWriter, r *http.Request) { adminWorkRoleAssignment(w, r, deps) })
	mux.HandleFunc("/api/v1/admin/devices", func(w http.ResponseWriter, r *http.Request) { adminDevices(w, r, deps) })
	mux.HandleFunc("/api/v1/admin/subjects", func(w http.ResponseWriter, r *http.Request) { adminSubjects(w, r, deps) })
	mux.HandleFunc("/api/v1/admin/audit-logs", func(w http.ResponseWriter, r *http.Request) { adminAuditLogs(w, r, deps) })
	mux.HandleFunc("/api/v1/admin/model-runs", func(w http.ResponseWriter, r *http.Request) { adminModelRun(w, r, deps) })
	mux.HandleFunc("/api/v1/admin/effectiveness", func(w http.ResponseWriter, r *http.Request) { adminEffectivenessReport(w, r, deps) })
	mux.HandleFunc("/api/v1/admin/effectiveness/subjects", func(w http.ResponseWriter, r *http.Request) { adminEffectivenessSubjects(w, r, deps) })
	mux.HandleFunc("/api/v1/admin/effectiveness/recompute", func(w http.ResponseWriter, r *http.Request) { adminEffectivenessRecompute(w, r, deps) })
	mux.HandleFunc("/api/v1/admin/effectiveness/summary", func(w http.ResponseWriter, r *http.Request) { adminEffectivenessSummary(w, r, deps) })
	mux.HandleFunc("/api/v1/admin/ai-interactions", func(w http.ResponseWriter, r *http.Request) { adminAIInteractions(w, r, deps) })
	mux.HandleFunc("/api/v1/admin/activities", func(w http.ResponseWriter, r *http.Request) { adminActivities(w, r, deps) })
	mux.HandleFunc("/api/v1/admin/rag/status", func(w http.ResponseWriter, r *http.Request) { adminRAGStatus(w, r, deps) })
	mux.HandleFunc("/api/v1/admin/rag/queries", func(w http.ResponseWriter, r *http.Request) { adminRAGQueries(w, r, deps) })
	mux.HandleFunc("/api/v1/admin/rag/queries/", func(w http.ResponseWriter, r *http.Request) { adminRAGQueries(w, r, deps) })
	mux.HandleFunc("/api/v1/admin/data-quality/summary", func(w http.ResponseWriter, r *http.Request) { adminDataQualitySummary(w, r, deps) })
	mux.HandleFunc("/api/v1/admin/data-quality/facts", func(w http.ResponseWriter, r *http.Request) { adminDataQualityFacts(w, r, deps) })
	mux.HandleFunc("/api/v1/admin/data-quality/recompute", func(w http.ResponseWriter, r *http.Request) { adminDataQualityRecompute(w, r, deps) })
	mux.HandleFunc("/api/v1/admin/browser-policy", func(w http.ResponseWriter, r *http.Request) { adminBrowserPolicy(w, r, deps) })
	mux.HandleFunc("/api/v1/admin/application-capture-policy", func(w http.ResponseWriter, r *http.Request) { adminApplicationPolicy(w, r, deps) })
	mux.HandleFunc("/api/v1/admin/projects", func(w http.ResponseWriter, r *http.Request) { adminProjects(w, r, deps) })
	mux.HandleFunc("/api/v1/admin/project-backfills", func(w http.ResponseWriter, r *http.Request) { adminProjectBackfills(w, r, deps) })
	mux.HandleFunc("/api/v1/admin/project-backfills/", func(w http.ResponseWriter, r *http.Request) { adminProjectBackfills(w, r, deps) })
	mux.HandleFunc("/api/v1/admin/adapter-health", func(w http.ResponseWriter, r *http.Request) { adminAdapterHealth(w, r, deps) })
	mux.HandleFunc("/api/v1/device/adapter-health", func(w http.ResponseWriter, r *http.Request) { deviceAdapterHealth(w, r, deps) })
	mux.HandleFunc("/api/v1/admin/work-episodes", func(w http.ResponseWriter, r *http.Request) { adminEpisodes(w, r, deps) })
	mux.HandleFunc("/api/v1/admin/work-episodes/", func(w http.ResponseWriter, r *http.Request) { adminEpisodes(w, r, deps) })
	if deps.StaticDir != "" {
		mux.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/", http.StatusPermanentRedirect)
		})
		mux.HandleFunc("/admin/", staticHandler(deps.StaticDir))
	}
	return requestMiddleware(mux)
}

func requestMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func login(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, 405, map[string]string{"error": "method_not_allowed"})
		return
	}
	var request struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := httpx.ReadJSON(r, 64*1024, &request); err != nil {
		httpx.WriteJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	session, err := deps.Auth.Login(r.Context(), request.Username, request.Password)
	if err != nil {
		writeInvalidCredentials(w)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "aetheris_session", Value: session.Token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil, Expires: session.ExpiresAt})
	http.SetCookie(w, &http.Cookie{Name: "aetheris_csrf", Value: session.CSRF, Path: "/", HttpOnly: false, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil, Expires: session.ExpiresAt})
	httpx.WriteJSON(w, 200, map[string]any{"user_id": session.Principal.ID, "tenant_id": session.Principal.TenantID, "username": session.Username, "access_role": session.AccessRole, "expires_at": session.ExpiresAt})
}

func writeInvalidCredentials(w http.ResponseWriter) {
	httpx.WriteJSON(w, http.StatusUnauthorized, map[string]string{
		"error":   "invalid_credentials",
		"message": "用户名或密码错误",
	})
}

func logout(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	if cookie, _ := r.Cookie("aetheris_session"); cookie != nil {
		_ = deps.Auth.Logout(r.Context(), cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: "aetheris_session", MaxAge: -1, Path: "/"})
	w.WriteHeader(http.StatusNoContent)
}
func refresh(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	withSession(w, r, deps, func(w http.ResponseWriter, _ *http.Request, p authorization.Principal) {
		httpx.WriteJSON(w, 200, map[string]any{"user_id": p.ID, "tenant_id": p.TenantID})
	})
}

func bootstrap(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, 405, map[string]string{"error": "method_not_allowed"})
		return
	}
	var request devices.BootstrapRequest
	if err := httpx.ReadJSON(r, 64*1024, &request); err != nil {
		httpx.WriteJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	credential, err := deps.Devices.Bootstrap(r.Context(), request)
	if err != nil {
		httpx.WriteJSON(w, 401, map[string]string{"error": "bootstrap_failed"})
		return
	}
	httpx.WriteJSON(w, 200, credential)
}

func heartbeat(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	principal, ok := devicePrincipal(w, r, deps)
	if !ok {
		return
	}
	status, err := deps.Devices.Heartbeat(r.Context(), principal)
	if err != nil {
		httpx.WriteJSON(w, 503, map[string]string{"error": "heartbeat_failed"})
		return
	}
	if deps.WorkRoles != nil {
		if role, roleErr := deps.WorkRoles.Resolve(r.Context(), principal.TenantID, principal.SubjectID, principal.DeviceID, "", time.Now().UTC()); roleErr == nil {
			status.WorkRole = role
		}
	}
	httpx.WriteJSON(w, 200, status)
}

func revokeDevice(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	principal, ok := devicePrincipal(w, r, deps)
	if !ok {
		return
	}
	if err := deps.Devices.RevokeCredential(r.Context(), principal); err != nil {
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "revoke_failed"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func ingest(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	principal, ok := devicePrincipal(w, r, deps)
	if !ok {
		return
	}
	var body struct {
		Events []json.RawMessage `json:"events"`
	}
	if err := httpx.ReadJSON(r, 10*1024*1024, &body); err != nil || len(body.Events) > 1000 {
		httpx.WriteJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	results := make([]map[string]any, 0, len(body.Events))
	for _, raw := range body.Events {
		event, err := events.ParseAndValidate(raw)
		if err != nil {
			results = append(results, map[string]any{"status": "rejected", "reason": err.Error()})
			continue
		}
		if event.TenantID != principal.TenantID || event.SubjectID != principal.SubjectID || event.DeviceID != principal.DeviceID {
			results = append(results, map[string]any{"event_id": event.EventID, "status": "rejected", "reason": "event identity does not match device credential"})
			continue
		}
		if deps.WorkRoles != nil {
			if role, roleErr := deps.WorkRoles.Resolve(r.Context(), principal.TenantID, principal.SubjectID, principal.DeviceID, event.ProjectID, time.Now().UTC()); roleErr == nil {
				event.WorkRole = &events.RoleSnapshot{RoleID: role.RoleID, Code: role.Code, Version: role.Version, Source: role.Source}
			}
		}
		status, err := deps.Events.Insert(r.Context(), principal.TenantID, event)
		result := map[string]any{"event_id": event.EventID, "status": status}
		if err != nil {
			result["status"] = "rejected"
			result["reason"] = "event storage failed"
		}
		results = append(results, result)
	}
	httpx.WriteJSON(w, 202, map[string]any{"results": results})
}

func adminEvents(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	withSession(w, r, deps, func(w http.ResponseWriter, r *http.Request, p authorization.Principal) {
		if err := authorization.Require(p, "events:read", authorization.Scope{TenantID: p.TenantID}); err != nil {
			httpx.WriteJSON(w, 403, map[string]string{"error": "forbidden"})
			return
		}
		filter := events.Filter{Limit: 100, ProjectID: r.URL.Query().Get("project_id"), DeviceID: r.URL.Query().Get("device_id"), SubjectID: r.URL.Query().Get("subject_id"), WorkRoleCode: r.URL.Query().Get("work_role"), EventType: r.URL.Query().Get("event_type")}
		result, err := deps.Events.List(r.Context(), events.Scope{TenantID: p.TenantID}, filter)
		if err != nil {
			httpx.WriteJSON(w, 500, map[string]string{"error": "query_failed"})
			return
		}
		for index := range result {
			result[index] = projectEventForAdmin(result[index])
		}
		httpx.WriteJSON(w, 200, map[string]any{"events": result, "count": len(result)})
	})
}

func adminEventMutation(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	withSession(w, r, deps, func(w http.ResponseWriter, r *http.Request, p authorization.Principal) {
		if r.Method == http.MethodGet {
			if err := authorization.Require(p, "events:read", authorization.Scope{TenantID: p.TenantID}); err != nil {
				httpx.WriteJSON(w, 403, map[string]string{"error": "forbidden"})
				return
			}
			eventID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/events/"), "/tombstone")
			event, err := deps.Events.Get(r.Context(), events.Scope{TenantID: p.TenantID}, eventID)
			if err != nil {
				httpx.WriteJSON(w, 404, map[string]string{"error": "event_not_found"})
				return
			}
			event = projectEventForAdmin(event)
			httpx.WriteJSON(w, 200, event)
			return
		}
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/tombstone") {
			httpx.WriteJSON(w, 404, map[string]string{"error": "not_found"})
			return
		}
		if err := authorization.Require(p, "events:delete", authorization.Scope{TenantID: p.TenantID}); err != nil {
			httpx.WriteJSON(w, 403, map[string]string{"error": "forbidden"})
			return
		}
		eventID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/events/"), "/tombstone")
		var body struct {
			Reason string `json:"reason"`
		}
		_ = httpx.ReadJSON(r, 64*1024, &body)
		if err := deps.Events.Tombstone(r.Context(), events.Scope{TenantID: p.TenantID}, eventID, body.Reason, p.ID); err != nil {
			httpx.WriteJSON(w, 500, map[string]string{"error": "tombstone_failed"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func adminSummary(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	withSession(w, r, deps, func(w http.ResponseWriter, r *http.Request, p authorization.Principal) {
		if err := authorization.Require(p, "events:read", authorization.Scope{TenantID: p.TenantID}); err != nil {
			httpx.WriteJSON(w, 403, map[string]string{"error": "forbidden"})
			return
		}
		var subjects, devices, eventsToday, activeDevices int
		if err := deps.Pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM subjects WHERE tenant_id=$1`, p.TenantID).Scan(&subjects); err != nil {
			httpx.WriteJSON(w, 500, map[string]string{"error": "query_failed"})
			return
		}
		if err := deps.Pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM devices WHERE tenant_id=$1`, p.TenantID).Scan(&devices); err != nil {
			httpx.WriteJSON(w, 500, map[string]string{"error": "query_failed"})
			return
		}
		if err := deps.Pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM events WHERE tenant_id=$1 AND occurred_at >= CURRENT_DATE`, p.TenantID).Scan(&eventsToday); err != nil {
			httpx.WriteJSON(w, 500, map[string]string{"error": "query_failed"})
			return
		}
		if err := deps.Pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM devices WHERE tenant_id=$1 AND last_seen_at >= NOW() - INTERVAL '15 minutes'`, p.TenantID).Scan(&activeDevices); err != nil {
			httpx.WriteJSON(w, 500, map[string]string{"error": "query_failed"})
			return
		}
		activity, err := deps.Events.List(r.Context(), events.Scope{TenantID: p.TenantID}, events.Filter{Limit: 8})
		if err != nil {
			httpx.WriteJSON(w, 500, map[string]string{"error": "query_failed"})
			return
		}
		httpx.WriteJSON(w, 200, map[string]any{"total_subjects": subjects, "total_devices": devices, "events_today": eventsToday, "active_devices": activeDevices, "activity": activity})
	})
}

func adminProviderHealth(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	withSession(w, r, deps, func(w http.ResponseWriter, r *http.Request, p authorization.Principal) {
		if err := authorization.Require(p, "events:read", authorization.Scope{TenantID: p.TenantID}); err != nil {
			httpx.WriteJSON(w, 403, map[string]string{"error": "forbidden"})
			return
		}
		status := "down"
		message := "未配置 Model Gateway"
		latency := int64(0)
		if deps.ModelGatewayURL != "" {
			status, message, latency = checkModelGateway(r.Context(), deps.ModelGatewayURL)
		}
		httpx.WriteJSON(w, 200, []map[string]any{{"name": "模型网关", "provider": "ollama/dify", "status": status, "latency_ms": latency, "checked_at": time.Now().UTC(), "message": message}})
	})
}

func checkModelGateway(ctx context.Context, baseURL string) (string, string, int64) {
	start := time.Now()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/readyz", nil)
	if err != nil {
		return "down", "模型网关地址无效", time.Since(start).Milliseconds()
	}
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return "down", "模型网关不可达", latency
	}
	defer response.Body.Close()
	var readiness struct {
		Status   string `json:"status"`
		Provider string `json:"provider"`
		Model    string `json:"model"`
		Error    string `json:"error"`
	}
	if json.NewDecoder(response.Body).Decode(&readiness) != nil {
		return "down", "模型网关就绪响应无效", latency
	}
	provider := readiness.Provider
	if provider == "ollama" {
		provider = "Ollama"
	} else if provider == "dify" {
		provider = "Dify"
	}
	if response.StatusCode < 400 && readiness.Status == "ready" {
		if readiness.Model != "" {
			return "healthy", provider + " 可用（" + readiness.Model + "）", latency
		}
		return "healthy", provider + " 可用", latency
	}
	if readiness.Error == "connection_error" {
		return "down", provider + " 连接失败", latency
	}
	if readiness.Error == "model_not_found" {
		return "down", provider + " 模型未安装", latency
	}
	return "down", provider + " 暂不可用", latency
}

func adminWorkRoles(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	withSession(w, r, deps, func(w http.ResponseWriter, r *http.Request, p authorization.Principal) {
		if err := authorization.Require(p, "events:read", authorization.Scope{TenantID: p.TenantID}); err != nil {
			httpx.WriteJSON(w, 403, map[string]string{"error": "forbidden"})
			return
		}
		if deps.Pool == nil {
			httpx.WriteJSON(w, 503, map[string]string{"error": "database_unavailable"})
			return
		}
		rows, err := deps.Pool.Query(r.Context(), `SELECT a.id, a.subject_id, COALESCE(a.project_id,''), r.code FROM work_role_assignments a JOIN work_roles r ON r.tenant_id=a.tenant_id AND r.id=a.work_role_id WHERE a.tenant_id=$1 ORDER BY a.created_at DESC`, p.TenantID)
		if err != nil {
			httpx.WriteJSON(w, 500, map[string]string{"error": "query_failed"})
			return
		}
		defer rows.Close()
		result := []map[string]any{}
		for rows.Next() {
			var id, subject, project, role string
			if rows.Scan(&id, &subject, &project, &role) == nil {
				result = append(result, map[string]any{"id": id, "subject_id": subject, "project_id": project, "role": role})
			}
		}
		httpx.WriteJSON(w, 200, result)
	})
}

func adminWorkRoleAssignment(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	withSession(w, r, deps, func(w http.ResponseWriter, r *http.Request, p authorization.Principal) {
		if r.Method != http.MethodPut {
			httpx.WriteJSON(w, 405, map[string]string{"error": "method_not_allowed"})
			return
		}
		if err := authorization.Require(p, "users:manage", authorization.Scope{TenantID: p.TenantID}); err != nil {
			httpx.WriteJSON(w, 403, map[string]string{"error": "forbidden"})
			return
		}
		var body struct {
			SubjectID string `json:"subject_id"`
			ProjectID string `json:"project_id"`
			Role      string `json:"role"`
		}
		if err := httpx.ReadJSON(r, 64*1024, &body); err != nil {
			httpx.WriteJSON(w, 400, map[string]string{"error": "invalid_request", "message": "角色分配请求格式错误"})
			return
		}
		body.SubjectID = strings.TrimSpace(body.SubjectID)
		body.ProjectID = strings.TrimSpace(body.ProjectID)
		body.Role = strings.TrimSpace(body.Role)
		if body.SubjectID == "" || (body.Role != "研发" && body.Role != "测试" && body.Role != "产品") {
			httpx.WriteJSON(w, 400, map[string]string{"error": "invalid_request", "message": "请选择有效的主体和工作角色"})
			return
		}
		if deps.Pool == nil {
			httpx.WriteJSON(w, 503, map[string]string{"error": "database_unavailable", "message": "数据库暂不可用"})
			return
		}

		tx, err := deps.Pool.Begin(r.Context())
		if err != nil {
			httpx.WriteJSON(w, 500, map[string]string{"error": "assignment_failed", "message": "角色分配失败，请稍后重试"})
			return
		}
		defer tx.Rollback(r.Context())

		var subjectExists bool
		if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM subjects WHERE tenant_id=$1 AND id=$2)`, p.TenantID, body.SubjectID).Scan(&subjectExists); err != nil {
			httpx.WriteJSON(w, 500, map[string]string{"error": "assignment_failed", "message": "角色分配失败，请稍后重试"})
			return
		}
		if !subjectExists {
			httpx.WriteJSON(w, 404, map[string]string{"error": "subject_not_found", "message": "所选主体不存在，请刷新后重试"})
			return
		}
		if body.ProjectID != "" {
			var projectExists bool
			if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM projects WHERE tenant_id=$1 AND id=$2)`, p.TenantID, body.ProjectID).Scan(&projectExists); err != nil {
				httpx.WriteJSON(w, 500, map[string]string{"error": "assignment_failed", "message": "角色分配失败，请稍后重试"})
				return
			}
			if !projectExists {
				httpx.WriteJSON(w, 404, map[string]string{"error": "project_not_found", "message": "所选项目不存在，请刷新后重试"})
				return
			}
		}

		var roleID string
		roleIDCandidate := "role-" + p.TenantID + "-" + body.Role
		err = tx.QueryRow(r.Context(), `INSERT INTO work_roles(id,tenant_id,code,display_name) VALUES($1,$2,$3,$3) ON CONFLICT(tenant_id,code,version) DO UPDATE SET display_name=EXCLUDED.display_name,active=TRUE RETURNING id`, roleIDCandidate, p.TenantID, body.Role).Scan(&roleID)
		assignmentID := "assignment-" + p.TenantID + "-" + body.SubjectID + "-" + body.ProjectID
		if err == nil {
			_, err = tx.Exec(r.Context(), `DELETE FROM work_role_assignments WHERE tenant_id=$1 AND subject_id=$2 AND project_id IS NOT DISTINCT FROM NULLIF($3,'') AND source='admin_assignment' AND id<>$4`, p.TenantID, body.SubjectID, body.ProjectID, assignmentID)
		}
		if err == nil {
			_, err = tx.Exec(r.Context(), `INSERT INTO work_role_assignments(id,tenant_id,work_role_id,subject_id,project_id,source,created_by) VALUES($1,$2,$3,$4,NULLIF($5,''),'admin_assignment',$6) ON CONFLICT(id) DO UPDATE SET work_role_id=EXCLUDED.work_role_id,project_id=EXCLUDED.project_id,valid_from=NOW(),valid_to=NULL`, assignmentID, p.TenantID, roleID, body.SubjectID, body.ProjectID, p.ID)
		}
		if err == nil {
			err = tx.Commit(r.Context())
		}
		if err != nil {
			httpx.WriteJSON(w, 500, map[string]string{"error": "assignment_failed", "message": "角色分配失败，请稍后重试"})
			return
		}
		httpx.WriteJSON(w, 200, map[string]any{"id": assignmentID, "subject_id": body.SubjectID, "project_id": body.ProjectID, "role": body.Role})
	})
}

func adminDevices(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	withSession(w, r, deps, func(w http.ResponseWriter, r *http.Request, p authorization.Principal) {
		if err := authorization.Require(p, "devices:manage", authorization.Scope{TenantID: p.TenantID}); err != nil {
			httpx.WriteJSON(w, 403, map[string]string{"error": "forbidden"})
			return
		}
		rows, err := deps.Pool.Query(r.Context(), `SELECT id,hostname,subject_id,status,last_seen_at FROM devices WHERE tenant_id=$1 ORDER BY last_seen_at DESC`, p.TenantID)
		if err != nil {
			httpx.WriteJSON(w, 500, map[string]string{"error": "query_failed"})
			return
		}
		defer rows.Close()
		result := []map[string]any{}
		for rows.Next() {
			var id, host, subject, status string
			var last time.Time
			if rows.Scan(&id, &host, &subject, &status, &last) == nil {
				result = append(result, map[string]any{"id": id, "name": host, "subject_id": subject, "status": status, "last_seen_at": last})
			}
		}
		httpx.WriteJSON(w, 200, result)
	})
}

func adminSubjects(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	withSession(w, r, deps, func(w http.ResponseWriter, r *http.Request, p authorization.Principal) {
		if err := authorization.Require(p, "events:read", authorization.Scope{TenantID: p.TenantID}); err != nil {
			httpx.WriteJSON(w, 403, map[string]string{"error": "forbidden"})
			return
		}
		rows, err := deps.Pool.Query(r.Context(), `SELECT s.id,s.display_name,COUNT(d.id) FROM subjects s LEFT JOIN devices d ON d.tenant_id=s.tenant_id AND d.subject_id=s.id WHERE s.tenant_id=$1 GROUP BY s.id,s.display_name ORDER BY s.display_name`, p.TenantID)
		if err != nil {
			httpx.WriteJSON(w, 500, map[string]string{"error": "query_failed"})
			return
		}
		defer rows.Close()
		result := []map[string]any{}
		for rows.Next() {
			var id, name string
			var count int
			if rows.Scan(&id, &name, &count) == nil {
				result = append(result, map[string]any{"id": id, "name": name, "device_count": count})
			}
		}
		httpx.WriteJSON(w, 200, result)
	})
}

func adminAuditLogs(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	withSession(w, r, deps, func(w http.ResponseWriter, r *http.Request, p authorization.Principal) {
		if err := authorization.Require(p, "audit:read", authorization.Scope{TenantID: p.TenantID}); err != nil {
			httpx.WriteJSON(w, 403, map[string]string{"error": "forbidden"})
			return
		}
		rows, err := deps.Pool.Query(r.Context(), `SELECT id,actor_id,action,resource_type,resource_id,outcome,created_at FROM audit_logs WHERE tenant_id=$1 ORDER BY created_at DESC LIMIT 200`, p.TenantID)
		if err != nil {
			httpx.WriteJSON(w, 500, map[string]string{"error": "query_failed"})
			return
		}
		defer rows.Close()
		result := []map[string]any{}
		for rows.Next() {
			var id int64
			var actor, action, resource, resourceID, outcome string
			var created time.Time
			if rows.Scan(&id, &actor, &action, &resource, &resourceID, &outcome, &created) == nil {
				result = append(result, map[string]any{"id": id, "actor": actor, "action": action, "resource": resource + ":" + resourceID, "result": outcome, "created_at": created})
			}
		}
		httpx.WriteJSON(w, 200, result)
	})
}

func adminModelRun(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	withSession(w, r, deps, func(w http.ResponseWriter, r *http.Request, p authorization.Principal) {
		if r.Method != http.MethodPost {
			httpx.WriteJSON(w, 405, map[string]string{"error": "method_not_allowed"})
			return
		}
		if err := authorization.Require(p, "models:invoke", authorization.Scope{TenantID: p.TenantID}); err != nil {
			httpx.WriteJSON(w, 403, map[string]string{"error": "forbidden"})
			return
		}
		var body struct {
			EventIDs []string `json:"input_event_ids"`
			Task     string   `json:"task"`
			Model    string   `json:"model"`
		}
		if err := httpx.ReadJSON(r, 64*1024, &body); err != nil || len(body.EventIDs) == 0 {
			httpx.WriteJSON(w, 400, map[string]string{"error": "invalid_request"})
			return
		}
		if deps.ModelGatewayURL == "" {
			httpx.WriteJSON(w, 503, map[string]string{"error": "model_gateway_disabled"})
			return
		}
		requestBody, _ := json.Marshal(map[string]any{"tenant_id": p.TenantID, "actor_id": p.ID, "task": body.Task, "model": body.Model, "input_event_ids": body.EventIDs})
		req, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, strings.TrimRight(deps.ModelGatewayURL, "/")+"/internal/v1/generate", bytes.NewReader(requestBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+deps.ModelGatewayToken)
		response, err := modelGatewayClient(deps.ModelGatewayTimeout).Do(req)
		if err != nil {
			httpx.WriteJSON(w, 502, map[string]string{"error": "model_gateway_unavailable"})
			return
		}
		defer response.Body.Close()
		var result map[string]any
		if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
			httpx.WriteJSON(w, 502, map[string]string{"error": "invalid_model_response"})
			return
		}
		if response.StatusCode >= 400 {
			httpx.WriteJSON(w, 502, map[string]any{"error": "model_gateway_error", "detail": result})
			return
		}
		httpx.WriteJSON(w, 202, result)
	})
}

func modelGatewayClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	return &http.Client{Timeout: timeout}
}

func devicePrincipal(w http.ResponseWriter, r *http.Request, deps Dependencies) (authorization.Principal, bool) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		httpx.WriteJSON(w, 401, map[string]string{"error": "unauthorized"})
		return authorization.Principal{}, false
	}
	principal, err := deps.Devices.ResolveCredential(r.Context(), strings.TrimPrefix(header, "Bearer "))
	if err != nil {
		httpx.WriteJSON(w, 401, map[string]string{"error": "unauthorized"})
		return authorization.Principal{}, false
	}
	return principal, true
}

func withSession(w http.ResponseWriter, r *http.Request, deps Dependencies, handler func(http.ResponseWriter, *http.Request, authorization.Principal)) {
	cookie, err := r.Cookie("aetheris_session")
	if err != nil {
		httpx.WriteJSON(w, 401, map[string]string{"error": "admin_login_required"})
		return
	}
	principal, _, err := deps.Auth.ResolveSession(r.Context(), cookie.Value)
	if err != nil {
		httpx.WriteJSON(w, 401, map[string]string{"error": "admin_login_required"})
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		csrfCookie, csrfErr := r.Cookie("aetheris_csrf")
		csrfHeader := r.Header.Get("X-CSRF-Token")
		if csrfErr != nil || csrfCookie.Value == "" || csrfHeader == "" || subtle.ConstantTimeCompare([]byte(csrfCookie.Value), []byte(csrfHeader)) != 1 {
			httpx.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "csrf_failed"})
			return
		}
	}
	handler(w, r, principal)
}

func staticHandler(root string) http.HandlerFunc {
	resolved, _ := filepath.Abs(root)
	return func(w http.ResponseWriter, r *http.Request) {
		relative := strings.TrimPrefix(r.URL.Path, "/admin/")
		if relative == "" {
			relative = "index.html"
		}
		candidate := filepath.Join(resolved, filepath.Clean(relative))
		if candidate != resolved && !strings.HasPrefix(candidate, resolved+string(os.PathSeparator)) {
			http.NotFound(w, r)
			return
		}
		if info, err := os.Stat(candidate); err != nil || info.IsDir() {
			candidate = filepath.Join(resolved, "index.html")
		}
		http.ServeFile(w, r, candidate)
	}
}

func serveClientDownload(w http.ResponseWriter, r *http.Request, configuredFile string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		httpx.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if configuredFile == "" {
		http.NotFound(w, r)
		return
	}
	info, err := os.Stat(configuredFile)
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(configuredFile)+`"`)
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, configuredFile)
}
