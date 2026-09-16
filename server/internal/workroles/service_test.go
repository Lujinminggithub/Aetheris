package workroles

import (
	"testing"
	"time"
)

func TestResolveRolePrefersProjectThenSubjectThenDefault(t *testing.T) {
	now := time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)
	assignments := []Assignment{
		{ID: "default", RoleID: "role-rnd", Code: "研发", Version: 1, Priority: 1, ValidFrom: now.Add(-time.Hour)},
		{ID: "subject", RoleID: "role-test", Code: "测试", Version: 1, Priority: 2, ValidFrom: now.Add(-time.Hour)},
		{ID: "project", RoleID: "role-product", Code: "产品", Version: 1, Priority: 3, ValidFrom: now.Add(-time.Hour)},
	}
	result, ok := ResolveAssignments(assignments, now)
	if !ok || result.Code != "产品" {
		t.Fatalf("role = %#v, ok=%v", result, ok)
	}
}

func TestResolveRoleIgnoresExpiredAssignment(t *testing.T) {
	now := time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)
	assignments := []Assignment{{ID: "expired", RoleID: "role-rnd", Code: "研发", Version: 1, Priority: 3, ValidFrom: now.Add(-2 * time.Hour), ValidTo: ptr(now.Add(-time.Hour))}}
	if _, ok := ResolveAssignments(assignments, now); ok {
		t.Fatal("expired assignment resolved")
	}
}

func ptr(value time.Time) *time.Time { return &value }
