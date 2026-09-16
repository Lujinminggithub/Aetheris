package retrieval

import "sync"

type WorkloadGate struct{ mutex sync.RWMutex }

func NewWorkloadGate() *WorkloadGate   { return &WorkloadGate{} }
func (gate *WorkloadGate) BeginIndex() { gate.mutex.RLock() }
func (gate *WorkloadGate) EndIndex()   { gate.mutex.RUnlock() }
func (gate *WorkloadGate) BeginQuery() { gate.mutex.Lock() }
func (gate *WorkloadGate) EndQuery()   { gate.mutex.Unlock() }
