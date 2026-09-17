package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aetheris-dev/aetheris/server/internal/authorization"
	"github.com/aetheris-dev/aetheris/server/internal/processknowledge"
)

type fakeProcessKnowledgeService struct {
	started processknowledge.Job
}

func (service *fakeProcessKnowledgeService) Summary(context.Context, string) (processknowledge.Summary, error) {
	return processknowledge.Summary{Sessions: 3, Units: 2, Verified: 1, Mode: "shadow"}, nil
}
func (service *fakeProcessKnowledgeService) ListUnits(context.Context, processknowledge.ListFilter) ([]processknowledge.KnowledgeUnit, int, error) {
	return []processknowledge.KnowledgeUnit{{KnowledgeDraft: processknowledge.KnowledgeDraft{ID: "knowledge-1", Topic: "Windows EDR", Conclusion: "内核与用户态协作", ValidationState: "verified"}}}, 1, nil
}
func (service *fakeProcessKnowledgeService) GetUnit(context.Context, string, string) (processknowledge.KnowledgeUnit, error) {
	return processknowledge.KnowledgeUnit{KnowledgeDraft: processknowledge.KnowledgeDraft{ID: "knowledge-1", Conclusion: "结论"}}, nil
}
func (service *fakeProcessKnowledgeService) StartBackfill(_ context.Context, tenant, actor, mode, project string, version int) (processknowledge.Job, error) {
	service.started = processknowledge.Job{ID: "job-1", TenantID: tenant, Mode: mode, LogicalProjectID: project, Version: version, State: "pending", CreatedBy: actor}
	return service.started, nil
}
func (service *fakeProcessKnowledgeService) GetJob(context.Context, string, string) (processknowledge.Job, error) {
	return service.started, nil
}
func (service *fakeProcessKnowledgeService) Activate(context.Context, string, int, string, int) error {
	return nil
}
func (service *fakeProcessKnowledgeService) Rollback(context.Context, string, int) error { return nil }

func TestProcessKnowledgeSummaryRequiresReadPermission(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/process-knowledge/summary", nil)
	forbidden := httptest.NewRecorder()
	serveAdminProcessKnowledge(forbidden, request, authorization.Principal{TenantID: "tenant", Permissions: map[string]bool{}}, Dependencies{ProcessKnowledge: &fakeProcessKnowledgeService{}})
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("status=%d", forbidden.Code)
	}
	allowed := httptest.NewRecorder()
	serveAdminProcessKnowledge(allowed, request, authorization.Principal{TenantID: "tenant", Permissions: map[string]bool{"process_knowledge:read": true}}, Dependencies{ProcessKnowledge: &fakeProcessKnowledgeService{}})
	if allowed.Code != http.StatusOK || !strings.Contains(allowed.Body.String(), `"verified":1`) {
		t.Fatalf("status=%d body=%s", allowed.Code, allowed.Body.String())
	}
}

func TestProcessKnowledgeBackfillRequiresManageAndKeepsLogicalProject(t *testing.T) {
	service := &fakeProcessKnowledgeService{}
	body := `{"mode":"apply","logical_project_id":"logical-safe","version":1}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/process-knowledge/backfills", strings.NewReader(body))
	response := httptest.NewRecorder()
	serveAdminProcessKnowledge(response, request, authorization.Principal{ID: "admin", TenantID: "tenant", Permissions: map[string]bool{"process_knowledge:manage": true}}, Dependencies{ProcessKnowledge: service})
	if response.Code != http.StatusAccepted || service.started.LogicalProjectID != "logical-safe" {
		t.Fatalf("status=%d job=%+v", response.Code, service.started)
	}
}
