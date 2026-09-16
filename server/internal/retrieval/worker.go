package retrieval

import (
	"context"
	"log"
	"time"
)

type Worker struct {
	indexer   *Indexer
	tenantID  string
	interval  time.Duration
	batchSize int
}

func NewWorker(indexer *Indexer, tenantID string, interval time.Duration, batchSize int) *Worker {
	return &Worker{indexer, tenantID, interval, batchSize}
}

func (worker *Worker) Run(ctx context.Context) {
	for {
		result, err := worker.indexer.RunOnce(ctx, worker.tenantID, worker.batchSize)
		if err != nil {
			log.Printf("retrieval indexing failed tenant=%s error=%T", worker.tenantID, err)
		} else if result.Indexed > 0 {
			log.Printf("retrieval indexing tenant=%s indexed=%d", worker.tenantID, result.Indexed)
		}
		if err == nil && continueImmediately(result, worker.batchSize) {
			select {
			case <-ctx.Done():
				return
			default:
				continue
			}
		}
		timer := time.NewTimer(worker.interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func continueImmediately(result IndexResult, batchSize int) bool {
	return batchSize > 0 && result.Indexed == batchSize
}
