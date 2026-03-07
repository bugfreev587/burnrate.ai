package provider

import (
	"sync"
	"time"
)

const (
	cooldownThreshold  = 3
	baseCooldownDur    = 30 * time.Second
	maxCooldownDur     = 5 * time.Minute
)

// DeploymentHealth tracks the health status of a single deployment.
type DeploymentHealth struct {
	mu            sync.RWMutex
	FailCount     int
	LastFailure   time.Time
	CooldownUntil time.Time
	cooldownLevel int // tracks exponential backoff level
}

// HealthTracker manages health status for all deployments.
type HealthTracker struct {
	mu      sync.RWMutex
	health  map[string]*DeploymentHealth
	nowFunc func() time.Time // for testing
}

func NewHealthTracker() *HealthTracker {
	return &HealthTracker{
		health:  make(map[string]*DeploymentHealth),
		nowFunc: time.Now,
	}
}

func (ht *HealthTracker) getOrCreate(deploymentID string) *DeploymentHealth {
	ht.mu.Lock()
	defer ht.mu.Unlock()
	h, ok := ht.health[deploymentID]
	if !ok {
		h = &DeploymentHealth{}
		ht.health[deploymentID] = h
	}
	return h
}

// RecordFailure records a failure for a deployment. After cooldownThreshold consecutive
// failures, the deployment enters cooldown with exponential backoff.
func (ht *HealthTracker) RecordFailure(deploymentID string) {
	h := ht.getOrCreate(deploymentID)
	h.mu.Lock()
	defer h.mu.Unlock()

	now := ht.nowFunc()
	h.FailCount++
	h.LastFailure = now

	if h.FailCount >= cooldownThreshold {
		h.cooldownLevel++
		dur := baseCooldownDur * (1 << (h.cooldownLevel - 1))
		if dur > maxCooldownDur {
			dur = maxCooldownDur
		}
		h.CooldownUntil = now.Add(dur)
		h.FailCount = 0 // reset so next round needs threshold failures again
	}
}

// RecordSuccess records a successful request, resetting the failure state.
func (ht *HealthTracker) RecordSuccess(deploymentID string) {
	h := ht.getOrCreate(deploymentID)
	h.mu.Lock()
	defer h.mu.Unlock()

	h.FailCount = 0
	h.cooldownLevel = 0
	h.CooldownUntil = time.Time{}
}

// IsHealthy returns true if the deployment is not in cooldown.
func (ht *HealthTracker) IsHealthy(deploymentID string) bool {
	ht.mu.RLock()
	h, ok := ht.health[deploymentID]
	ht.mu.RUnlock()
	if !ok {
		return true
	}

	h.mu.RLock()
	defer h.mu.RUnlock()
	return ht.nowFunc().After(h.CooldownUntil)
}

// EarliestCooldownExpiry returns the deployment ID with the earliest cooldown expiry
// among the given deployment IDs. Used when all deployments are in cooldown.
func (ht *HealthTracker) EarliestCooldownExpiry(deploymentIDs []string) string {
	ht.mu.RLock()
	defer ht.mu.RUnlock()

	var earliest string
	var earliestTime time.Time

	for _, id := range deploymentIDs {
		h, ok := ht.health[id]
		if !ok {
			return id // no health record = healthy
		}
		h.mu.RLock()
		cu := h.CooldownUntil
		h.mu.RUnlock()

		if earliest == "" || cu.Before(earliestTime) {
			earliest = id
			earliestTime = cu
		}
	}
	return earliest
}
