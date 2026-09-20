package publicknowledge

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
	batch     int
}

func NewWorker(processor Processor, interval time.Duration, batch int) *Worker {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	if batch < 1 {
		batch = 25
	}
	return &Worker{processor: processor, interval: interval, batch: batch}
}

func (worker *Worker) RunOnce(ctx context.Context) (bool, error) {
	return worker.processor.ProcessNext(ctx, worker.batch)
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
