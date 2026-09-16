package cleaning

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
	ticker := time.NewTicker(worker.interval)
	defer ticker.Stop()
	for {
		now := time.Now().In(worker.service.Location())
		day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, worker.service.Location())
		if _, err := worker.service.RunRange(ctx, worker.tenantID, day.AddDate(0, 0, -1), day, CurrentRuleVersion); err != nil {
			log.Printf("cleaning recompute failed tenant=%s error=%v", worker.tenantID, err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
