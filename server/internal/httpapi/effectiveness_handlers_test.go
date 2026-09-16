package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aetheris-dev/aetheris/server/internal/authorization"
	"github.com/aetheris-dev/aetheris/server/internal/effectiveness"
)

func TestEffectivenessSummaryContextExcludesVerboseEvidenceAndDailyRows(t *testing.T) {
	report := effectiveness.Report{
		SubjectID: "subject-1", From: "2026-09-01", To: "2026-09-07",
		Totals:           effectiveness.Totals{ActiveDays: 3, ActiveWindowMinutes: 120},
		Daily:            []effectiveness.DailyMetrics{{LocalDate: "2026-09-07", EvidenceEventIDs: []string{"event-daily"}}},
		EvidenceEventIDs: []string{"event-top-level"},
		Definitions:      map[string]string{"active_window_minutes": "不等同于工时"},
	}

	encoded, err := json.Marshal(effectivenessSummaryContext(report))
	if err != nil {
		t.Fatal(err)
	}
	var context map[string]any
	if err := json.Unmarshal(encoded, &context); err != nil {
		t.Fatal(err)
	}
	if _, exists := context["daily"]; exists {
		t.Fatal("summary context must not contain daily rows")
	}
	if _, exists := context["evidence_event_ids"]; exists {
		t.Fatal("summary context must not contain evidence ids")
	}
	if context["totals"] == nil || context["coverage"] == nil || context["definitions"] == nil {
		t.Fatalf("summary context lost explainable metrics: %#v", context)
	}
}

func TestEffectivenessReportRequiresSubject(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/effectiveness", nil)
	response := httptest.NewRecorder()
	serveEffectivenessReport(response, request, authorization.Principal{Kind: "user", TenantID: "tenant-1"}, Dependencies{})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestEffectivenessReportRejectsMoreThanNinetyDays(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/effectiveness?subject_id=s1&from=2026-01-01&to=2026-04-01", nil)
	response := httptest.NewRecorder()
	service := effectiveness.NewService(nil, nil, time.UTC)
	serveEffectivenessReport(response, request, authorization.Principal{Kind: "user", TenantID: "tenant-1", Permissions: map[string]bool{"effectiveness:read": true}}, Dependencies{Effectiveness: service})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestEffectivenessReportRejectsDevicePrincipal(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/effectiveness?subject_id=s1", nil)
	response := httptest.NewRecorder()
	serveEffectivenessReport(response, request, authorization.Principal{Kind: "device", TenantID: "tenant-1", Permissions: map[string]bool{"effectiveness:read": true}}, Dependencies{})
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestEffectivenessReportRequiresPermission(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/effectiveness?subject_id=s1", nil)
	response := httptest.NewRecorder()
	service := effectiveness.NewService(nil, nil, time.UTC)
	serveEffectivenessReport(response, request, authorization.Principal{Kind: "user", TenantID: "tenant-1", Permissions: map[string]bool{}}, Dependencies{Effectiveness: service})
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d", response.Code)
	}
}
