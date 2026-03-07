package provider

import (
	"sync"
	"time"
)

const latencyWindowSize = 100

// LatencyTracker maintains a sliding window of request latencies per deployment.
type LatencyTracker struct {
	mu      sync.RWMutex
	windows map[string]*latencyWindow
}

type latencyWindow struct {
	durations []time.Duration
	pos       int
	full      bool
}

func NewLatencyTracker() *LatencyTracker {
	return &LatencyTracker{
		windows: make(map[string]*latencyWindow),
	}
}

// Record adds a latency measurement for the given deployment.
func (lt *LatencyTracker) Record(deploymentID string, duration time.Duration) {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	w, ok := lt.windows[deploymentID]
	if !ok {
		w = &latencyWindow{
			durations: make([]time.Duration, latencyWindowSize),
		}
		lt.windows[deploymentID] = w
	}

	w.durations[w.pos] = duration
	w.pos = (w.pos + 1) % latencyWindowSize
	if w.pos == 0 {
		w.full = true
	}
}

// Average returns the average latency for the given deployment.
// Returns 0 if no data is available.
func (lt *LatencyTracker) Average(deploymentID string) time.Duration {
	lt.mu.RLock()
	defer lt.mu.RUnlock()

	w, ok := lt.windows[deploymentID]
	if !ok {
		return 0
	}

	count := w.pos
	if w.full {
		count = latencyWindowSize
	}
	if count == 0 {
		return 0
	}

	var total time.Duration
	for i := 0; i < count; i++ {
		total += w.durations[i]
	}
	return total / time.Duration(count)
}

// HasData returns true if there is at least one latency measurement for the deployment.
func (lt *LatencyTracker) HasData(deploymentID string) bool {
	lt.mu.RLock()
	defer lt.mu.RUnlock()
	w, ok := lt.windows[deploymentID]
	if !ok {
		return false
	}
	return w.full || w.pos > 0
}
