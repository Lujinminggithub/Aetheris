package retrieval

import (
	"testing"
	"time"
)

func TestWorkloadGateGivesQueryExclusiveAccess(t *testing.T) {
	gate := NewWorkloadGate()
	gate.BeginIndex()
	queryStarted := make(chan struct{})
	queryRelease := make(chan struct{})
	go func() { gate.BeginQuery(); close(queryStarted); <-queryRelease; gate.EndQuery() }()
	select {
	case <-queryStarted:
		t.Fatal("query bypassed active index batch")
	case <-time.After(20 * time.Millisecond):
	}
	gate.EndIndex()
	select {
	case <-queryStarted:
	case <-time.After(time.Second):
		t.Fatal("query did not acquire gate")
	}
	indexStarted := make(chan struct{})
	go func() { gate.BeginIndex(); close(indexStarted); gate.EndIndex() }()
	select {
	case <-indexStarted:
		t.Fatal("index bypassed active query")
	case <-time.After(20 * time.Millisecond):
	}
	close(queryRelease)
	select {
	case <-indexStarted:
	case <-time.After(time.Second):
		t.Fatal("index did not resume")
	}
}
