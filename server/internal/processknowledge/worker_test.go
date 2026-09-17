package processknowledge

import (
	"context"
	"testing"
	"time"
)

func TestWorkerRunOnceDelegatesBoundedBatch(t *testing.T) {
	store := &fakeKnowledgeStore{}
	worker := NewWorker(store, time.Second, 25)
	didWork, err := worker.RunOnce(context.Background())
	if err != nil || !didWork || store.processCalls != 1 {
		t.Fatalf("didWork=%v calls=%d err=%v", didWork, store.processCalls, err)
	}
}
