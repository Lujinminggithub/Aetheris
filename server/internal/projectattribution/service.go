package projectattribution

import (
	"context"
	"fmt"
)

type Store interface {
	CreateJob(context.Context, string, string, string, int) (Job, error)
	GetJob(context.Context, string, string) (Job, error)
	Activate(context.Context, string, int) error
	Rollback(context.Context, string, int) error
}

type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }

func (service *Service) Start(ctx context.Context, tenantID, actorID, mode string, ruleVersion int) (Job, error) {
	if tenantID == "" || actorID == "" {
		return Job{}, fmt.Errorf("历史归属任务身份无效")
	}
	if mode != "dry_run" && mode != "apply" {
		return Job{}, fmt.Errorf("历史归属任务模式无效")
	}
	if ruleVersion < 1 {
		return Job{}, fmt.Errorf("历史归属规则版本无效")
	}
	return service.store.CreateJob(ctx, tenantID, actorID, mode, ruleVersion)
}

func (service *Service) Get(ctx context.Context, tenantID, jobID string) (Job, error) {
	job, err := service.store.GetJob(ctx, tenantID, jobID)
	if err != nil {
		return Job{}, err
	}
	if job.ID == "" || job.TenantID != tenantID {
		return Job{}, fmt.Errorf("历史归属任务不存在")
	}
	return job, nil
}

func (service *Service) Activate(ctx context.Context, tenantID, jobID string) error {
	job, err := service.Get(ctx, tenantID, jobID)
	if err != nil {
		return err
	}
	if job.Mode != "apply" || job.State != "completed" {
		return fmt.Errorf("只有已完成的正式回填任务可以启用")
	}
	return service.store.Activate(ctx, tenantID, job.RuleVersion)
}

func (service *Service) Rollback(ctx context.Context, tenantID string, ruleVersion int) error {
	if tenantID == "" || ruleVersion < 1 {
		return fmt.Errorf("回滚规则版本无效")
	}
	return service.store.Rollback(ctx, tenantID, ruleVersion)
}
