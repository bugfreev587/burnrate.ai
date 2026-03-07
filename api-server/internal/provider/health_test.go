package provider

import (
	"testing"
	"time"
)

func TestHealthTracker_InitiallyHealthy(t *testing.T) {
	ht := NewHealthTracker()
	if !ht.IsHealthy("deploy-1") {
		t.Error("new deployment should be healthy")
	}
}

func TestHealthTracker_CooldownAfterThreshold(t *testing.T) {
	now := time.Now()
	ht := NewHealthTracker()
	ht.nowFunc = func() time.Time { return now }

	// Record failures below threshold — should remain healthy
	ht.RecordFailure("deploy-1")
	ht.RecordFailure("deploy-1")
	if !ht.IsHealthy("deploy-1") {
		t.Error("should be healthy below threshold")
	}

	// Third failure triggers cooldown
	ht.RecordFailure("deploy-1")
	if ht.IsHealthy("deploy-1") {
		t.Error("should be in cooldown after 3 failures")
	}
}

func TestHealthTracker_CooldownExpiry(t *testing.T) {
	now := time.Now()
	ht := NewHealthTracker()
	ht.nowFunc = func() time.Time { return now }

	// Trigger cooldown
	for i := 0; i < 3; i++ {
		ht.RecordFailure("deploy-1")
	}
	if ht.IsHealthy("deploy-1") {
		t.Error("should be in cooldown")
	}

	// Advance past cooldown (30s base)
	now = now.Add(31 * time.Second)
	if !ht.IsHealthy("deploy-1") {
		t.Error("should be healthy after cooldown expires")
	}
}

func TestHealthTracker_ExponentialBackoff(t *testing.T) {
	now := time.Now()
	ht := NewHealthTracker()
	ht.nowFunc = func() time.Time { return now }

	// First cooldown: 30s (level 1)
	for i := 0; i < 3; i++ {
		ht.RecordFailure("deploy-1")
	}
	if ht.IsHealthy("deploy-1") {
		t.Error("should be in 1st cooldown")
	}

	// Advance past first cooldown
	now = now.Add(31 * time.Second)
	if !ht.IsHealthy("deploy-1") {
		t.Error("should be healthy after 1st cooldown expires")
	}

	// More failures → 2nd cooldown: 60s (level 2)
	for i := 0; i < 3; i++ {
		ht.RecordFailure("deploy-1")
	}
	if ht.IsHealthy("deploy-1") {
		t.Error("should be in 2nd cooldown")
	}

	// 31s is not enough for 60s cooldown
	now = now.Add(31 * time.Second)
	if ht.IsHealthy("deploy-1") {
		t.Error("should still be in cooldown (60s)")
	}

	// 61s total from 2nd cooldown start is enough
	now = now.Add(30 * time.Second)
	if !ht.IsHealthy("deploy-1") {
		t.Error("should be healthy after 2nd cooldown expires")
	}
}

func TestHealthTracker_SuccessResetsState(t *testing.T) {
	now := time.Now()
	ht := NewHealthTracker()
	ht.nowFunc = func() time.Time { return now }

	// Record some failures (below threshold)
	ht.RecordFailure("deploy-1")
	ht.RecordFailure("deploy-1")

	// Success resets
	ht.RecordSuccess("deploy-1")

	// Need 3 new failures to trigger cooldown
	ht.RecordFailure("deploy-1")
	ht.RecordFailure("deploy-1")
	if !ht.IsHealthy("deploy-1") {
		t.Error("should be healthy — only 2 failures after reset")
	}
}

func TestHealthTracker_MaxCooldown(t *testing.T) {
	now := time.Now()
	ht := NewHealthTracker()
	ht.nowFunc = func() time.Time { return now }

	// Trigger many rounds of cooldown
	for round := 0; round < 10; round++ {
		for i := 0; i < 3; i++ {
			ht.RecordFailure("deploy-1")
		}
		now = now.Add(10 * time.Minute) // advance past any cooldown
	}

	// Verify cooldown doesn't exceed max (5 min)
	h := ht.health["deploy-1"]
	h.mu.RLock()
	cooldownDur := h.CooldownUntil.Sub(now)
	h.mu.RUnlock()
	// The last cooldown was set from the last failure time, which is `now` before the last Add
	// After the loop, the health should still have a cooldown <= maxCooldownDur from the last failure
	if cooldownDur > maxCooldownDur {
		t.Errorf("cooldown %v exceeds max %v", cooldownDur, maxCooldownDur)
	}
}

func TestHealthTracker_EarliestCooldownExpiry(t *testing.T) {
	now := time.Now()
	ht := NewHealthTracker()
	ht.nowFunc = func() time.Time { return now }

	// Put deploy-1 in cooldown
	for i := 0; i < 3; i++ {
		ht.RecordFailure("deploy-1")
	}

	// Advance and put deploy-2 in cooldown (later expiry)
	now = now.Add(10 * time.Second)
	for i := 0; i < 3; i++ {
		ht.RecordFailure("deploy-2")
	}

	earliest := ht.EarliestCooldownExpiry([]string{"deploy-1", "deploy-2"})
	if earliest != "deploy-1" {
		t.Errorf("expected deploy-1 (earliest cooldown), got %s", earliest)
	}
}

func TestHealthTracker_EarliestCooldownExpiry_UnknownDeployment(t *testing.T) {
	ht := NewHealthTracker()

	// deploy-3 has no health record — should be returned as "healthy"
	earliest := ht.EarliestCooldownExpiry([]string{"deploy-3"})
	if earliest != "deploy-3" {
		t.Errorf("expected deploy-3 (no health record = healthy), got %s", earliest)
	}
}

func TestHealthTracker_MultipleDeployments(t *testing.T) {
	ht := NewHealthTracker()

	// Only deploy-1 has failures
	for i := 0; i < 3; i++ {
		ht.RecordFailure("deploy-1")
	}

	if ht.IsHealthy("deploy-1") {
		t.Error("deploy-1 should be in cooldown")
	}
	if !ht.IsHealthy("deploy-2") {
		t.Error("deploy-2 should be healthy (no interactions)")
	}
}
