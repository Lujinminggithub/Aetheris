package processknowledge

import (
	"context"
	"fmt"
)

type Store interface {
	CreateJob(context.Context, string, string, string, string, int) (Job, error)
	GetJob(context.Context, string, string) (Job, error)
	Activate(context.Context, string, int, string, int) error
	Rollback(context.Context, string, int) error
	Summary(context.Context, string) (Summary, error)
	ListUnits(context.Context, ListFilter) ([]KnowledgeUnit, int, error)
	GetUnit(context.Context, string, string) (KnowledgeUnit, error)
	ProcessNext(context.Context, int) (bool, error)
}

type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }

func (service *Service) StartBackfill(ctx context.Context, tenantID, actorID, mode, projectID string, version int) (Job, error) {
	if mode != "dry_run" && mode != "apply" {
		return Job{}, fmt.Errorf("回填模式必须是 dry_run 或 apply")
	}
	if version < 1 {
		return Job{}, fmt.Errorf("知识版本必须为正整数")
	}
	return service.store.CreateJob(ctx, tenantID, actorID, mode, projectID, version)
}

func (service *Service) GetJob(ctx context.Context, tenantID, id string) (Job, error) {
	return service.store.GetJob(ctx, tenantID, id)
}

func (service *Service) Activate(ctx context.Context, tenantID string, version int, mode string, canaryPercent int) error {
	if version < 1 || (mode != "shadow" && mode != "canary" && mode != "active") {
		return fmt.Errorf("知识版本或运行模式无效")
	}
	if mode != "canary" {
		canaryPercent = 0
	}
	if canaryPercent < 0 || canaryPercent > 100 {
		return fmt.Errorf("灰度比例必须在 0 到 100 之间")
	}
	return service.store.Activate(ctx, tenantID, version, mode, canaryPercent)
}

func (service *Service) Rollback(ctx context.Context, tenantID string, version int) error {
	if version < 1 {
		return fmt.Errorf("知识版本必须为正整数")
	}
	return service.store.Rollback(ctx, tenantID, version)
}

func (service *Service) Summary(ctx context.Context, tenantID string) (Summary, error) {
	return service.store.Summary(ctx, tenantID)
}

func (service *Service) ListUnits(ctx context.Context, filter ListFilter) ([]KnowledgeUnit, int, error) {
	if filter.Limit < 1 || filter.Limit > 200 {
		filter.Limit = 50
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	return service.store.ListUnits(ctx, filter)
}

func (service *Service) GetUnit(ctx context.Context, tenantID, id string) (KnowledgeUnit, error) {
	return service.store.GetUnit(ctx, tenantID, id)
}
