package effectiveness

import (
	"context"
	"log"
	"time"
)

type Worker struct {
	service  *Service
	tenantID string
	interval time.Duration
}

func NewWorker(service *Service, tenantID string, interval time.Duration) *Worker {
	return &Worker{service: service, tenantID: tenantID, interval: interval}
}

func (worker *Worker) Run(ctx context.Context) {
	worker.recomputeRecent(ctx)
	ticker := time.NewTicker(worker.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			worker.recomputeRecent(ctx)
		}
	}
}

func (worker *Worker) recomputeRecent(ctx context.Context) {
	now := time.Now().In(worker.service.location)
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, worker.service.location).AddDate(0, 0, -1)
	if _, err := worker.service.RecomputeRange(ctx, worker.tenantID, from, from.AddDate(0, 0, 1)); err != nil {
		log.Printf("effectiveness recompute failed tenant=%s error=%v", worker.tenantID, err)
	}
}
