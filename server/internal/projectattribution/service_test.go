package projectattribution

import (
	"context"
	"testing"
)

type memoryJobStore struct {
	jobs       map[string]Job
	activated  int
	rolledBack int
}

func (store *memoryJobStore) CreateJob(_ context.Context, tenantID, actorID, mode string, ruleVersion int) (Job, error) {
	job := Job{ID: "job-1", TenantID: tenantID, CreatedBy: actorID, Mode: mode, RuleVersion: ruleVersion, State: "pending"}
	store.jobs[job.ID] = job
	return job, nil
}
func (store *memoryJobStore) GetJob(_ context.Context, tenantID, jobID string) (Job, error) {
	return store.jobs[jobID], nil
}
func (store *memoryJobStore) Activate(_ context.Context, _ string, ruleVersion int) error {
	store.activated = ruleVersion
	return nil
}
func (store *memoryJobStore) Rollback(_ context.Context, _ string, ruleVersion int) error {
	store.rolledBack = ruleVersion
	return nil
}

func TestServiceValidatesModeAndRuleVersion(t *testing.T) {
	store := &memoryJobStore{jobs: map[string]Job{}}
	service := NewService(store)
	if _, err := service.Start(context.Background(), "tenant-1", "admin-1", "unknown", 1); err == nil {
		t.Fatal("expected invalid mode error")
	}
	if _, err := service.Start(context.Background(), "tenant-1", "admin-1", "apply", 0); err == nil {
		t.Fatal("expected invalid rule version error")
	}
	job, err := service.Start(context.Background(), "tenant-1", "admin-1", "dry_run", 2)
	if err != nil || job.Mode != "dry_run" || job.RuleVersion != 2 {
		t.Fatalf("unexpected job=%+v err=%v", job, err)
	}
}

func TestServiceOnlyActivatesCompletedApplyJob(t *testing.T) {
	store := &memoryJobStore{jobs: map[string]Job{
		"dry":     {ID: "dry", TenantID: "tenant-1", Mode: "dry_run", RuleVersion: 2, State: "completed"},
		"running": {ID: "running", TenantID: "tenant-1", Mode: "apply", RuleVersion: 3, State: "running"},
		"ready":   {ID: "ready", TenantID: "tenant-1", Mode: "apply", RuleVersion: 4, State: "completed"},
	}}
	service := NewService(store)
	if err := service.Activate(context.Background(), "tenant-1", "dry"); err == nil {
		t.Fatal("dry run must not activate")
	}
	if err := service.Activate(context.Background(), "tenant-1", "running"); err == nil {
		t.Fatal("running job must not activate")
	}
	if err := service.Activate(context.Background(), "tenant-1", "ready"); err != nil {
		t.Fatal(err)
	}
	if store.activated != 4 {
		t.Fatalf("activated version=%d", store.activated)
	}
}
