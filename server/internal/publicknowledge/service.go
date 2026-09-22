package publicknowledge

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrRevisionConflict = errors.New("public knowledge revision conflict")
var ErrInvalidTransition = errors.New("invalid public knowledge transition")
var ErrNotFound = errors.New("public knowledge not found")

type Store interface {
	Summary(context.Context) (Summary, error)
	List(context.Context, ListFilter) ([]Unit, int, error)
	Get(context.Context, string, string) (Unit, error)
	ApplyReview(context.Context, ReviewCommand, Transition) (Unit, error)
	CreateJob(context.Context, string, string) (Job, error)
	GetJob(context.Context, string) (Job, error)
}

type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }

func (service *Service) Summary(ctx context.Context) (Summary, error) {
	return service.store.Summary(ctx)
}

func (service *Service) List(ctx context.Context, filter ListFilter) ([]Unit, int, error) {
	if filter.Limit < 1 || filter.Limit > 200 {
		filter.Limit = 50
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	return service.store.List(ctx, filter)
}

func (service *Service) Get(ctx context.Context, id, viewerTenantID string) (Unit, error) {
	return service.store.Get(ctx, id, viewerTenantID)
}

func (service *Service) Confirm(ctx context.Context, command ReviewCommand) (Unit, error) {
	return service.review(ctx, command, "confirm", []PublicationState{CandidateState, PendingReview}, SourceConfirmed, PendingReview)
}

func (service *Service) Verify(ctx context.Context, command ReviewCommand) (Unit, error) {
	return service.review(ctx, command, "verify", []PublicationState{CandidateState, PendingReview}, EvidenceVerified, PendingReview)
}

func (service *Service) Certify(ctx context.Context, command ReviewCommand) (Unit, error) {
	unit, err := service.preflight(ctx, command)
	if err != nil {
		return Unit{}, err
	}
	if unit.PublicationState != PendingReview || unit.ScopeState != ScopeClassified || len(unit.Domains) == 0 || !substantiveReviewReason(command.Reason) || (unit.Revision.ValidationState != SourceConfirmed && unit.Revision.ValidationState != EvidenceVerified && unit.Revision.ValidationState != CrossTenantCorroborated) {
		return Unit{}, ErrInvalidTransition
	}
	command.Action = "certify"
	return service.store.ApplyReview(ctx, command, Transition{PublicationState: PendingReview, ValidationState: PlatformCertified})
}

func substantiveReviewReason(value string) bool {
	normalized := strings.TrimSpace(strings.ToLower(value))
	if len([]rune(normalized)) < 20 {
		return false
	}
	for _, weak := range []string{"ok", "通过", "同意", "可以", "已确认"} {
		if normalized == weak {
			return false
		}
	}
	return true
}

func (service *Service) Reject(ctx context.Context, command ReviewCommand) (Unit, error) {
	return service.review(ctx, command, "reject", []PublicationState{CandidateState, PendingReview}, Contradicted, Withdrawn)
}

func (service *Service) Suspend(ctx context.Context, command ReviewCommand) (Unit, error) {
	unit, err := service.preflight(ctx, command)
	if err != nil {
		return Unit{}, err
	}
	if unit.PublicationState != Published {
		return Unit{}, ErrInvalidTransition
	}
	command.Action = "suspend"
	return service.store.ApplyReview(ctx, command, Transition{PublicationState: Suspended, ValidationState: unit.Revision.ValidationState})
}

func (service *Service) Withdraw(ctx context.Context, command ReviewCommand) (Unit, error) {
	unit, err := service.preflight(ctx, command)
	if err != nil {
		return Unit{}, err
	}
	if unit.PublicationState == Withdrawn {
		return unit, nil
	}
	command.Action = "withdraw"
	return service.store.ApplyReview(ctx, command, Transition{PublicationState: Withdrawn, ValidationState: unit.Revision.ValidationState})
}

func (service *Service) Republish(ctx context.Context, command ReviewCommand) (Unit, error) {
	unit, err := service.preflight(ctx, command)
	if err != nil {
		return Unit{}, err
	}
	if unit.PublicationState != Suspended || unit.Revision.ValidationState != PlatformCertified {
		return Unit{}, ErrInvalidTransition
	}
	command.Action = "republish"
	return service.store.ApplyReview(ctx, command, Transition{PublicationState: PendingReview, ValidationState: PlatformCertified})
}

func (service *Service) StartBuild(ctx context.Context, actorID, mode string) (Job, error) {
	if mode != "build_candidates" && mode != "reindex" && mode != "rebuild" {
		return Job{}, fmt.Errorf("公共知识任务模式无效")
	}
	return service.store.CreateJob(ctx, actorID, mode)
}

func (service *Service) GetJob(ctx context.Context, id string) (Job, error) {
	return service.store.GetJob(ctx, id)
}

func (service *Service) review(ctx context.Context, command ReviewCommand, action string, allowed []PublicationState, validation ValidationState, publication PublicationState) (Unit, error) {
	unit, err := service.preflight(ctx, command)
	if err != nil {
		return Unit{}, err
	}
	allowedState := false
	for _, state := range allowed {
		allowedState = allowedState || unit.PublicationState == state
	}
	if !allowedState {
		return Unit{}, ErrInvalidTransition
	}
	command.Action = action
	return service.store.ApplyReview(ctx, command, Transition{PublicationState: publication, ValidationState: validation})
}

func (service *Service) preflight(ctx context.Context, command ReviewCommand) (Unit, error) {
	if command.KnowledgeID == "" || command.ExpectedRevision < 1 || command.ActorID == "" || strings.TrimSpace(command.Reason) == "" {
		return Unit{}, ErrInvalidTransition
	}
	unit, err := service.store.Get(ctx, command.KnowledgeID, command.ActorTenantID)
	if err != nil {
		return Unit{}, err
	}
	if unit.CurrentRevision != command.ExpectedRevision {
		return Unit{}, ErrRevisionConflict
	}
	return unit, nil
}
