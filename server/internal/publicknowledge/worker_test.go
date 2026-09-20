package publicknowledge

import (
	"context"
	"testing"
	"time"
)

type fakeProcessor struct {
	batch int
	did   bool
}

func (processor *fakeProcessor) ProcessNext(_ context.Context, batch int) (bool, error) {
	processor.batch = batch
	return processor.did, nil
}

func TestWorkerRunOnceDelegatesBoundedBatch(t *testing.T) {
	processor := &fakeProcessor{did: true}
	worker := NewWorker(processor, time.Second, 25)
	didWork, err := worker.RunOnce(context.Background())
	if err != nil || !didWork || processor.batch != 25 {
		t.Fatalf("didWork=%v batch=%d err=%v", didWork, processor.batch, err)
	}
}
