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

type drainingProcessor struct {
	calls int
	done  chan struct{}
}

func (processor *drainingProcessor) ProcessNext(context.Context, int) (bool, error) {
	processor.calls++
	if processor.calls == 3 {
		close(processor.done)
		return false, nil
	}
	return true, nil
}

func TestWorkerDrainsPendingBatchesWithoutWaitingForIdleInterval(t *testing.T) {
	processor := &drainingProcessor{done: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go NewWorker(processor, time.Hour, 25).Run(ctx)
	select {
	case <-processor.done:
		if processor.calls != 3 {
			t.Fatalf("calls=%d", processor.calls)
		}
	case <-time.After(time.Second):
		t.Fatal("pending batches waited for the idle interval")
	}
}
