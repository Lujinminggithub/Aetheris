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
	for {
		didWork, _ := worker.RunOnce(ctx)
		delay := worker.interval
		if didWork {
			delay = 100 * time.Millisecond
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
