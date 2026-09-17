package processknowledge

import (
	"context"
	"time"
)

type Processor interface {
	ProcessNext(context.Context, int) (bool, error)
}

type Worker struct {
	processor Processor
	interval  time.Duration
	batchSize int
}

func NewWorker(processor Processor, interval time.Duration, batchSize int) *Worker {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	if batchSize < 1 {
		batchSize = 100
	}
	return &Worker{processor: processor, interval: interval, batchSize: batchSize}
}

func (worker *Worker) RunOnce(ctx context.Context) (bool, error) {
	return worker.processor.ProcessNext(ctx, worker.batchSize)
}

func (worker *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(worker.interval)
	defer ticker.Stop()
	for {
		_, _ = worker.RunOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
