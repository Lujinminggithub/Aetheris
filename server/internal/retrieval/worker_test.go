package retrieval

import "testing"

func TestWorkerContinuesImmediatelyForFullBatch(t *testing.T) {
	if !continueImmediately(IndexResult{Indexed: 64}, 64) {
		t.Fatal("full batch did not continue")
	}
	if continueImmediately(IndexResult{Indexed: 63}, 64) {
		t.Fatal("partial batch skipped idle wait")
	}
}
