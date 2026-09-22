package publicknowledge

import (
	"context"
	"errors"
	"testing"
)

type fakeStore struct {
	unit       Unit
	transition Transition
	err        error
}

func (store *fakeStore) Summary(context.Context) (Summary, error) {
	return Summary{Published: 2}, store.err
}

func (store *fakeStore) List(context.Context, ListFilter) ([]Unit, int, error) {
	return []Unit{store.unit}, 1, store.err
}
func (store *fakeStore) Get(context.Context, string, string) (Unit, error) {
	return store.unit, store.err
}
func (store *fakeStore) ApplyReview(_ context.Context, _ ReviewCommand, transition Transition) (Unit, error) {
	store.transition = transition
	if store.err != nil {
		return Unit{}, store.err
	}
	store.unit.PublicationState = transition.PublicationState
	store.unit.Revision.ValidationState = transition.ValidationState
	return store.unit, nil
}
func (store *fakeStore) CreateJob(context.Context, string, string) (Job, error) {
	return Job{}, store.err
}
func (store *fakeStore) GetJob(context.Context, string) (Job, error) { return Job{}, store.err }

func TestCertifyRequiresPriorEvidenceAndWaitsForIndex(t *testing.T) {
	store := &fakeStore{unit: Unit{ID: "public-1", PublicationState: PendingReview, CurrentRevision: 2, ScopeState: ScopeClassified, Domains: []string{"dlp"}, Revision: Revision{Revision: 2, ValidationState: EvidenceVerified}}}
	service := NewService(store)
	unit, err := service.Certify(context.Background(), ReviewCommand{KnowledgeID: "public-1", ExpectedRevision: 2, ActorID: "admin", Action: "certify", Reason: "已核对领域、实体、适用条件和验证证据，允许发布"})
	if err != nil {
		t.Fatal(err)
	}
	if unit.PublicationState != PendingReview || unit.Revision.ValidationState != PlatformCertified {
		t.Fatalf("unit=%+v", unit)
	}
	if store.transition.PublicationState == Published {
		t.Fatal("certification must not publish before vector indexing")
	}
}

func TestCertifyRejectsUnverifiedKnowledge(t *testing.T) {
	store := &fakeStore{unit: Unit{ID: "public-1", PublicationState: CandidateState, CurrentRevision: 1, Revision: Revision{Revision: 1, ValidationState: Unverified}}}
	_, err := NewService(store).Certify(context.Background(), ReviewCommand{KnowledgeID: "public-1", ExpectedRevision: 1, ActorID: "admin", Reason: "直接发布"})
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("err=%v", err)
	}
}

func TestWithdrawIsIdempotentAndRevisionChecked(t *testing.T) {
	store := &fakeStore{unit: Unit{ID: "public-1", PublicationState: Published, CurrentRevision: 3, Revision: Revision{Revision: 3, ValidationState: PlatformCertified}}}
	service := NewService(store)
	if _, err := service.Withdraw(context.Background(), ReviewCommand{KnowledgeID: "public-1", ExpectedRevision: 2, ActorID: "admin", Reason: "来源撤回"}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("err=%v", err)
	}
	if _, err := service.Withdraw(context.Background(), ReviewCommand{KnowledgeID: "public-1", ExpectedRevision: 3, ActorID: "admin", Reason: "来源撤回"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Withdraw(context.Background(), ReviewCommand{KnowledgeID: "public-1", ExpectedRevision: 3, ActorID: "admin", Reason: "重复撤回"}); err != nil {
		t.Fatal(err)
	}
}

func TestCertifyRequiresClassifiedScopeAndSubstantiveReason(t *testing.T) {
	store := &fakeStore{unit: Unit{ID: "public-1", PublicationState: PendingReview, CurrentRevision: 1, ScopeState: ScopeClassified, Domains: []string{"dlp"}, Revision: Revision{Revision: 1, ValidationState: EvidenceVerified}}}
	service := NewService(store)
	if _, err := service.Certify(context.Background(), ReviewCommand{KnowledgeID: "public-1", ExpectedRevision: 1, ActorID: "admin", Reason: "ok"}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("short review accepted: %v", err)
	}
	store.unit.ScopeState, store.unit.Domains = ScopeUnclassified, nil
	if _, err := service.Certify(context.Background(), ReviewCommand{KnowledgeID: "public-1", ExpectedRevision: 1, ActorID: "admin", Reason: "已核对领域、实体、适用条件和验证证据，允许发布"}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("unclassified scope accepted: %v", err)
	}
}
