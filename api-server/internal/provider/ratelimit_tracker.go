package provider

import (
	"sync"
)

// RateLimitTracker stores per-deployment rate limit info from response headers.
type RateLimitTracker struct {
	mu    sync.RWMutex
	state map[string]*RateLimitInfo
}

func NewRateLimitTracker() *RateLimitTracker {
	return &RateLimitTracker{
		state: make(map[string]*RateLimitInfo),
	}
}

// Update stores rate limit info for a deployment.
func (rlt *RateLimitTracker) Update(deploymentID string, info *RateLimitInfo) {
	if info == nil {
		return
	}
	rlt.mu.Lock()
	defer rlt.mu.Unlock()
	rlt.state[deploymentID] = info
}

// ShouldAvoid returns true if the deployment's remaining capacity is below 10%
// of its known limit, indicating it should be proactively avoided.
func (rlt *RateLimitTracker) ShouldAvoid(deploymentID string) bool {
	rlt.mu.RLock()
	defer rlt.mu.RUnlock()

	info, ok := rlt.state[deploymentID]
	if !ok {
		return false // no data = assume healthy
	}

	// Check requests
	if info.LimitRequests > 0 {
		threshold := info.LimitRequests / 10
		if info.RemainingRequests < threshold {
			return true
		}
	}

	// Check tokens
	if info.LimitTokens > 0 {
		threshold := info.LimitTokens / 10
		if info.RemainingTokens < threshold {
			return true
		}
	}

	return false
}

// Get returns the current rate limit info for a deployment.
func (rlt *RateLimitTracker) Get(deploymentID string) *RateLimitInfo {
	rlt.mu.RLock()
	defer rlt.mu.RUnlock()
	return rlt.state[deploymentID]
}
