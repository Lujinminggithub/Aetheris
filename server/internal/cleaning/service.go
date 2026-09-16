package cleaning

import (
	"context"
	"time"

	"github.com/aetheris-dev/aetheris/server/internal/auth"
)

type Service struct {
	repository *Repository
	location   *time.Location
}

func NewService(repository *Repository, location *time.Location) *Service {
	return &Service{repository: repository, location: location}
}
func (service *Service) Location() *time.Location { return service.location }

func (service *Service) RecomputeRange(ctx context.Context, tenantID string, from, to time.Time, version int) (string, error) {
	if err := ValidateRange(from, to); err != nil {
		return "", err
	}
	token, err := auth.RandomToken(12)
	if err != nil {
		return "", err
	}
	jobID := "cleaning-" + token
	if err := service.repository.CreateJob(ctx, jobID, tenantID, from, to, version); err != nil {
		return "", err
	}
	go service.run(context.Background(), jobID, tenantID, from, to, version)
	return jobID, nil
}

func (service *Service) RunRange(ctx context.Context, tenantID string, from, to time.Time, version int) ([]Fact, error) {
	evidence, err := service.repository.ListEvidence(ctx, tenantID, from, to.AddDate(0, 0, 1))
	if err != nil {
		return nil, err
	}
	facts := NormalizeEvents(evidence, version)
	if err := service.repository.ReplaceRange(ctx, tenantID, version, from, to.AddDate(0, 0, 1), facts); err != nil {
		return nil, err
	}
	return facts, nil
}

func (service *Service) run(ctx context.Context, jobID, tenantID string, from, to time.Time, version int) {
	service.repository.UpdateJob(ctx, jobID, "running", "", 0, nil)
	evidence, err := service.repository.ListEvidence(ctx, tenantID, from, to.AddDate(0, 0, 1))
	if err != nil {
		service.repository.UpdateJob(ctx, jobID, "failed", "evidence_query_failed", 0, nil)
		return
	}
	facts := NormalizeEvents(evidence, version)
	if err := service.repository.ReplaceRange(ctx, tenantID, version, from, to.AddDate(0, 0, 1), facts); err != nil {
		service.repository.UpdateJob(ctx, jobID, "failed", "fact_write_failed", len(evidence), facts)
		return
	}
	service.repository.UpdateJob(ctx, jobID, "completed", "", len(evidence), facts)
}

func (service *Service) Summary(ctx context.Context, tenantID string, from, to time.Time) (Summary, error) {
	return service.repository.Summary(ctx, tenantID, from, to.AddDate(0, 0, 1), CurrentRuleVersion)
}

func (service *Service) ListFacts(ctx context.Context, tenantID string, from, to time.Time, limit, offset int, qualityState string) (FactPage, error) {
	return service.repository.ListFacts(ctx, tenantID, from, to.AddDate(0, 0, 1), CurrentRuleVersion, limit, offset, qualityState)
}
