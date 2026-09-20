package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aetheris-dev/aetheris/server/internal/authorization"
	"github.com/aetheris-dev/aetheris/server/internal/publicknowledge"
)

type fakePublicKnowledgeService struct {
	command publicknowledge.ReviewCommand
	err     error
}

func (service *fakePublicKnowledgeService) Summary(context.Context) (publicknowledge.Summary, error) {
	return publicknowledge.Summary{Published: 2, Pending: 1}, service.err
}
func (service *fakePublicKnowledgeService) List(context.Context, publicknowledge.ListFilter) ([]publicknowledge.Unit, int, error) {
	return []publicknowledge.Unit{{ID: "public-1", PublicationState: publicknowledge.Published}}, 1, service.err
}
func (service *fakePublicKnowledgeService) Get(context.Context, string, string) (publicknowledge.Unit, error) {
	return publicknowledge.Unit{ID: "public-1", CurrentRevision: 2}, service.err
}
func (service *fakePublicKnowledgeService) Confirm(ctx context.Context, command publicknowledge.ReviewCommand) (publicknowledge.Unit, error) {
	return service.review(command)
}
func (service *fakePublicKnowledgeService) Verify(ctx context.Context, command publicknowledge.ReviewCommand) (publicknowledge.Unit, error) {
	return service.review(command)
}
func (service *fakePublicKnowledgeService) Certify(ctx context.Context, command publicknowledge.ReviewCommand) (publicknowledge.Unit, error) {
	return service.review(command)
}
func (service *fakePublicKnowledgeService) Reject(ctx context.Context, command publicknowledge.ReviewCommand) (publicknowledge.Unit, error) {
	return service.review(command)
}
func (service *fakePublicKnowledgeService) Suspend(ctx context.Context, command publicknowledge.ReviewCommand) (publicknowledge.Unit, error) {
	return service.review(command)
}
func (service *fakePublicKnowledgeService) Withdraw(ctx context.Context, command publicknowledge.ReviewCommand) (publicknowledge.Unit, error) {
	return service.review(command)
}
func (service *fakePublicKnowledgeService) Republish(ctx context.Context, command publicknowledge.ReviewCommand) (publicknowledge.Unit, error) {
	return service.review(command)
}
func (service *fakePublicKnowledgeService) StartBuild(context.Context, string, string) (publicknowledge.Job, error) {
	return publicknowledge.Job{ID: "job-1", State: "pending"}, service.err
}
func (service *fakePublicKnowledgeService) GetJob(context.Context, string) (publicknowledge.Job, error) {
	return publicknowledge.Job{ID: "job-1", State: "completed"}, service.err
}
func (service *fakePublicKnowledgeService) review(command publicknowledge.ReviewCommand) (publicknowledge.Unit, error) {
	service.command = command
	return publicknowledge.Unit{ID: command.KnowledgeID, CurrentRevision: command.ExpectedRevision}, service.err
}

func TestPublicKnowledgeSummaryRequiresDiagnosePermission(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/public-knowledge/summary", nil)
	forbidden := httptest.NewRecorder()
	serveAdminPublicKnowledge(forbidden, request, authorization.Principal{TenantID: "tenant-a", Permissions: map[string]bool{}}, Dependencies{PublicKnowledge: &fakePublicKnowledgeService{}})
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("status=%d", forbidden.Code)
	}
	allowed := httptest.NewRecorder()
	serveAdminPublicKnowledge(allowed, request, authorization.Principal{TenantID: "tenant-a", Permissions: map[string]bool{"knowledge:diagnose": true}}, Dependencies{PublicKnowledge: &fakePublicKnowledgeService{}})
	if allowed.Code != http.StatusOK || !strings.Contains(allowed.Body.String(), `"published":2`) {
		t.Fatalf("status=%d body=%s", allowed.Code, allowed.Body.String())
	}
}

func TestPublicKnowledgeCertificationRequiresPlatformPermissionAndRevision(t *testing.T) {
	body := `{"expected_revision":2,"reason":"证据和适用条件已复核"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/public-knowledge/units/public-1/certify", strings.NewReader(body))
	forbidden := httptest.NewRecorder()
	serveAdminPublicKnowledge(forbidden, request, authorization.Principal{ID: "tenant-admin", TenantID: "tenant-a", Permissions: map[string]bool{"knowledge:verify": true}}, Dependencies{PublicKnowledge: &fakePublicKnowledgeService{}})
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("status=%d", forbidden.Code)
	}
	service := &fakePublicKnowledgeService{}
	allowed := httptest.NewRecorder()
	serveAdminPublicKnowledge(allowed, request, authorization.Principal{ID: "platform-admin", TenantID: "tenant-a", Permissions: map[string]bool{"knowledge:certify_public": true}}, Dependencies{PublicKnowledge: service})
	if allowed.Code != http.StatusOK || service.command.KnowledgeID != "public-1" || service.command.ExpectedRevision != 2 || service.command.ActorTenantID != "tenant-a" {
		t.Fatalf("status=%d command=%+v body=%s", allowed.Code, service.command, allowed.Body.String())
	}
}

func TestPublicKnowledgeRevisionConflictReturns409(t *testing.T) {
	service := &fakePublicKnowledgeService{err: publicknowledge.ErrRevisionConflict}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/public-knowledge/units/public-1/verify", strings.NewReader(`{"expected_revision":1,"reason":"验证"}`))
	response := httptest.NewRecorder()
	serveAdminPublicKnowledge(response, request, authorization.Principal{ID: "analyst", TenantID: "tenant-a", Permissions: map[string]bool{"knowledge:verify": true}}, Dependencies{PublicKnowledge: service})
	if response.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
