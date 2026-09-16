package projectattribution

import (
	"context"
	"time"
)

type Worker struct {
	repository *Repository
	interval   time.Duration
	batchSize  int
}

func NewWorker(repository *Repository, interval time.Duration, batchSize int) *Worker {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if batchSize < 1 {
		batchSize = 10000
	}
	return &Worker{repository: repository, interval: interval, batchSize: batchSize}
}

func (worker *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(worker.interval)
	defer ticker.Stop()
	for {
		_, _ = worker.repository.ProcessNext(ctx, worker.batchSize)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
