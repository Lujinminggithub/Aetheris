package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aetheris-dev/aetheris/server/internal/authorization"
	"github.com/aetheris-dev/aetheris/server/internal/projectattribution"
)

type fakeBackfillService struct {
	job             projectattribution.Job
	activatedJobID  string
	rollbackVersion int
}

func (service *fakeBackfillService) Start(_ context.Context, tenantID, actorID, mode string, ruleVersion int) (projectattribution.Job, error) {
	service.job = projectattribution.Job{ID: "job-1", TenantID: tenantID, CreatedBy: actorID, Mode: mode, RuleVersion: ruleVersion, State: "pending"}
	return service.job, nil
}
func (service *fakeBackfillService) Get(_ context.Context, _, _ string) (projectattribution.Job, error) {
	return service.job, nil
}
func (service *fakeBackfillService) Activate(_ context.Context, _ string, jobID string) error {
	service.activatedJobID = jobID
	return nil
}
func (service *fakeBackfillService) Rollback(_ context.Context, _ string, ruleVersion int) error {
	service.rollbackVersion = ruleVersion
	return nil
}

func TestAdminProjectBackfillLifecycle(t *testing.T) {
	service := &fakeBackfillService{}
	principal := authorization.Principal{Kind: "user", ID: "admin-1", TenantID: "tenant-1", Permissions: map[string]bool{"projects:manage": true}}

	create := httptest.NewRecorder()
	serveAdminProjectBackfills(create, httptest.NewRequest(http.MethodPost, "/api/v1/admin/project-backfills", strings.NewReader(`{"mode":"apply","rule_version":2}`)), principal, Dependencies{ProjectBackfills: service})
	if create.Code != http.StatusAccepted || !strings.Contains(create.Body.String(), `"id":"job-1"`) {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}

	service.job.State = "completed"
	get := httptest.NewRecorder()
	serveAdminProjectBackfills(get, httptest.NewRequest(http.MethodGet, "/api/v1/admin/project-backfills/job-1", nil), principal, Dependencies{ProjectBackfills: service})
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), `"state":"completed"`) {
		t.Fatalf("get status=%d body=%s", get.Code, get.Body.String())
	}

	activate := httptest.NewRecorder()
	serveAdminProjectBackfills(activate, httptest.NewRequest(http.MethodPost, "/api/v1/admin/project-backfills/job-1/activate", nil), principal, Dependencies{ProjectBackfills: service})
	if activate.Code != http.StatusNoContent || service.activatedJobID != "job-1" {
		t.Fatalf("activate status=%d job=%s", activate.Code, service.activatedJobID)
	}

	rollback := httptest.NewRecorder()
	serveAdminProjectBackfills(rollback, httptest.NewRequest(http.MethodPost, "/api/v1/admin/project-backfills/rollback", strings.NewReader(`{"rule_version":1}`)), principal, Dependencies{ProjectBackfills: service})
	if rollback.Code != http.StatusNoContent || service.rollbackVersion != 1 {
		t.Fatalf("rollback status=%d version=%d", rollback.Code, service.rollbackVersion)
	}
}

func TestAdminProjectBackfillRequiresPermission(t *testing.T) {
	response := httptest.NewRecorder()
	serveAdminProjectBackfills(response, httptest.NewRequest(http.MethodPost, "/api/v1/admin/project-backfills", strings.NewReader(`{"mode":"dry_run","rule_version":1}`)), authorization.Principal{TenantID: "tenant-1", Permissions: map[string]bool{}}, Dependencies{ProjectBackfills: &fakeBackfillService{}})
	if response.Code != http.StatusForbidden {
		t.Fatalf("status=%d", response.Code)
	}
}
