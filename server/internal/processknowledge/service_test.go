package processknowledge

import (
	"context"
	"errors"
	"testing"
)

type fakeKnowledgeStore struct {
	job          Job
	active       int
	rolledBack   int
	processCalls int
}

func (store *fakeKnowledgeStore) CreateJob(_ context.Context, tenantID, actorID, mode, projectID string, version int) (Job, error) {
	store.job = Job{ID: "knowledge-job", TenantID: tenantID, CreatedBy: actorID, Mode: mode, LogicalProjectID: projectID, Version: version, State: "pending"}
	return store.job, nil
}
func (store *fakeKnowledgeStore) GetJob(context.Context, string, string) (Job, error) {
	return store.job, nil
}
func (store *fakeKnowledgeStore) Activate(_ context.Context, _ string, version int, _ string, _ int) error {
	store.active = version
	return nil
}
func (store *fakeKnowledgeStore) Rollback(_ context.Context, _ string, version int) error {
	store.rolledBack = version
	return nil
}
func (store *fakeKnowledgeStore) Summary(context.Context, string) (Summary, error) {
	return Summary{Sessions: 2}, nil
}
func (store *fakeKnowledgeStore) ListUnits(context.Context, ListFilter) ([]KnowledgeUnit, int, error) {
	return []KnowledgeUnit{}, 0, nil
}
func (store *fakeKnowledgeStore) GetUnit(context.Context, string, string) (KnowledgeUnit, error) {
	return KnowledgeUnit{}, nil
}
func (store *fakeKnowledgeStore) ProcessNext(context.Context, int) (bool, error) {
	store.processCalls++
	return true, nil
}

func TestServiceValidatesBackfillAndVersionTransitions(t *testing.T) {
	store := &fakeKnowledgeStore{}
	service := NewService(store)
	if _, err := service.StartBackfill(context.Background(), "tenant", "actor", "invalid", "project", 1); err == nil {
		t.Fatal("invalid mode accepted")
	}
	if _, err := service.StartBackfill(context.Background(), "tenant", "actor", "apply", "", 0); err == nil {
		t.Fatal("invalid version accepted")
	}
	job, err := service.StartBackfill(context.Background(), "tenant", "actor", "dry_run", "logical-safe", 2)
	if err != nil || job.LogicalProjectID != "logical-safe" {
		t.Fatalf("job=%+v err=%v", job, err)
	}
	if err := service.Activate(context.Background(), "tenant", 2, "canary", 20); err != nil || store.active != 2 {
		t.Fatalf("activate err=%v version=%d", err, store.active)
	}
	if err := service.Activate(context.Background(), "tenant", 2, "invalid", 0); err == nil {
		t.Fatal("invalid mode accepted")
	}
	if err := service.Rollback(context.Background(), "tenant", 1); err != nil || store.rolledBack != 1 {
		t.Fatalf("rollback err=%v version=%d", err, store.rolledBack)
	}
}

type failingStore struct{ fakeKnowledgeStore }

func (store *failingStore) ProcessNext(context.Context, int) (bool, error) {
	return true, errors.New("boom")
}
