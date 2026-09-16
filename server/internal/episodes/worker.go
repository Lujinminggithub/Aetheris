package episodes

import (
	"context"
	"net/http"
	"time"
)

type Worker struct {
	repository   *Repository
	tenantID     string
	interval     time.Duration
	ollamaClient *http.Client
	ollamaURL    string
	ollamaModel  string
}

func (worker *Worker) WithOllama(client *http.Client, endpoint, model string) *Worker {
	worker.ollamaClient, worker.ollamaURL, worker.ollamaModel = client, endpoint, model
	return worker
}

func NewWorker(repository *Repository, tenantID string, interval time.Duration) *Worker {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &Worker{repository: repository, tenantID: tenantID, interval: interval}
}

func (worker *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(worker.interval)
	defer ticker.Stop()
	for {
		_, _ = worker.repository.RecomputeRecentWithEnrichment(ctx, worker.tenantID, time.Now().UTC().Add(-24*time.Hour), worker.ollamaClient, worker.ollamaURL, worker.ollamaModel)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
