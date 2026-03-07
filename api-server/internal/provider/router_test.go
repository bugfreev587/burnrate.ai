package provider

import (
	"context"
	"testing"
	"time"
)

func TestRegistry_BuiltInAdapters(t *testing.T) {
	r := NewRegistry()

	oai, err := r.Get("openai")
	if err != nil {
		t.Fatalf("expected openai adapter: %v", err)
	}
	if oai.Name() != "openai" {
		t.Error("wrong adapter name")
	}

	ant, err := r.Get("anthropic")
	if err != nil {
		t.Fatalf("expected anthropic adapter: %v", err)
	}
	if ant.Name() != "anthropic" {
		t.Error("wrong adapter name")
	}
}

func TestRegistry_UnknownProvider(t *testing.T) {
	r := NewRegistry()
	_, err := r.Get("gemini")
	if err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestGetStrategy(t *testing.T) {
	strategies := []string{"fallback", "round-robin", "lowest-latency", "cost-optimized"}
	for _, name := range strategies {
		t.Run(name, func(t *testing.T) {
			s, err := GetStrategy(name)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if s == nil {
				t.Fatal("expected non-nil strategy")
			}
		})
	}

	_, err := GetStrategy("unknown")
	if err == nil {
		t.Error("expected error for unknown strategy")
	}
}

func TestFallbackStrategy_PriorityOrder(t *testing.T) {
	state := NewRouterState()
	group := &ModelGroup{
		Name:     "test",
		Strategy: "fallback",
		Deployments: []Deployment{
			{ID: "low-priority", Provider: "openai", Priority: 10},
			{ID: "high-priority", Provider: "openai", Priority: 1},
			{ID: "mid-priority", Provider: "openai", Priority: 5},
		},
	}

	strategy := &FallbackStrategy{}
	dep, err := strategy.Select(context.Background(), group, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dep.ID != "high-priority" {
		t.Errorf("expected high-priority deployment, got %s", dep.ID)
	}
}

func TestFallbackStrategy_SkipsCooldown(t *testing.T) {
	state := NewRouterState()

	// Put high-priority in cooldown
	for i := 0; i < 3; i++ {
		state.Health.RecordFailure("high-priority")
	}

	group := &ModelGroup{
		Name:     "test",
		Strategy: "fallback",
		Deployments: []Deployment{
			{ID: "high-priority", Provider: "openai", Priority: 1},
			{ID: "low-priority", Provider: "openai", Priority: 10},
		},
	}

	strategy := &FallbackStrategy{}
	dep, err := strategy.Select(context.Background(), group, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dep.ID != "low-priority" {
		t.Errorf("expected low-priority (fallback), got %s", dep.ID)
	}
}

func TestFallbackStrategy_AllInCooldown(t *testing.T) {
	now := time.Now()
	state := NewRouterState()
	state.Health.nowFunc = func() time.Time { return now }

	// Put both in cooldown, high-priority first (will expire first)
	for i := 0; i < 3; i++ {
		state.Health.RecordFailure("first")
	}
	now = now.Add(5 * time.Second)
	for i := 0; i < 3; i++ {
		state.Health.RecordFailure("second")
	}

	group := &ModelGroup{
		Name:     "test",
		Strategy: "fallback",
		Deployments: []Deployment{
			{ID: "second", Provider: "openai", Priority: 1},
			{ID: "first", Provider: "openai", Priority: 10},
		},
	}

	strategy := &FallbackStrategy{}
	dep, err := strategy.Select(context.Background(), group, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dep.ID != "first" {
		t.Errorf("expected 'first' (earliest cooldown expiry), got %s", dep.ID)
	}
}

func TestRoundRobinStrategy_EvenDistribution(t *testing.T) {
	state := NewRouterState()
	group := &ModelGroup{
		Name:     "test",
		Strategy: "round-robin",
		Deployments: []Deployment{
			{ID: "a", Provider: "openai"},
			{ID: "b", Provider: "openai"},
			{ID: "c", Provider: "openai"},
		},
	}

	strategy := &RoundRobinStrategy{}
	counts := make(map[string]int)

	for i := 0; i < 30; i++ {
		dep, err := strategy.Select(context.Background(), group, state)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		counts[dep.ID]++
	}

	// Each should get exactly 10
	for _, id := range []string{"a", "b", "c"} {
		if counts[id] != 10 {
			t.Errorf("expected 10 requests for %s, got %d", id, counts[id])
		}
	}
}

func TestRoundRobinStrategy_WeightedDistribution(t *testing.T) {
	state := NewRouterState()
	group := &ModelGroup{
		Name:     "test",
		Strategy: "round-robin",
		Deployments: []Deployment{
			{ID: "heavy", Provider: "openai", Weight: 3},
			{ID: "light", Provider: "openai", Weight: 1},
		},
	}

	strategy := &RoundRobinStrategy{}
	counts := make(map[string]int)

	for i := 0; i < 40; i++ {
		dep, err := strategy.Select(context.Background(), group, state)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		counts[dep.ID]++
	}

	// heavy should get ~3x more
	if counts["heavy"] < counts["light"] {
		t.Errorf("expected heavy (%d) > light (%d)", counts["heavy"], counts["light"])
	}
}

func TestCostOptimizedStrategy(t *testing.T) {
	state := NewRouterState()
	group := &ModelGroup{
		Name:     "test",
		Strategy: "cost-optimized",
		Deployments: []Deployment{
			{ID: "expensive", Provider: "openai", CostPer1KInput: 10.0, CostPer1KOutput: 30.0},
			{ID: "cheap", Provider: "openai", CostPer1KInput: 1.0, CostPer1KOutput: 2.0},
			{ID: "medium", Provider: "openai", CostPer1KInput: 5.0, CostPer1KOutput: 15.0},
		},
	}

	strategy := &CostOptimizedStrategy{}
	dep, err := strategy.Select(context.Background(), group, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dep.ID != "cheap" {
		t.Errorf("expected cheapest deployment, got %s", dep.ID)
	}
}

func TestLatencyTracker(t *testing.T) {
	lt := NewLatencyTracker()

	if lt.HasData("deploy-1") {
		t.Error("should not have data initially")
	}

	lt.Record("deploy-1", 100*time.Millisecond)
	lt.Record("deploy-1", 200*time.Millisecond)

	if !lt.HasData("deploy-1") {
		t.Error("should have data after recording")
	}

	avg := lt.Average("deploy-1")
	if avg != 150*time.Millisecond {
		t.Errorf("expected 150ms avg, got %v", avg)
	}
}

func TestLatencyTracker_SlidingWindow(t *testing.T) {
	lt := NewLatencyTracker()

	// Fill the window
	for i := 0; i < latencyWindowSize; i++ {
		lt.Record("deploy-1", 100*time.Millisecond)
	}

	// Overwrite with faster values
	for i := 0; i < latencyWindowSize; i++ {
		lt.Record("deploy-1", 50*time.Millisecond)
	}

	avg := lt.Average("deploy-1")
	if avg != 50*time.Millisecond {
		t.Errorf("expected 50ms avg after window overwrite, got %v", avg)
	}
}

func TestRateLimitTracker(t *testing.T) {
	rlt := NewRateLimitTracker()

	// No data = don't avoid
	if rlt.ShouldAvoid("deploy-1") {
		t.Error("should not avoid unknown deployment")
	}

	// Healthy rate limits
	rlt.Update("deploy-1", &RateLimitInfo{
		RemainingRequests: 50,
		LimitRequests:     100,
		RemainingTokens:   5000,
		LimitTokens:       10000,
	})
	if rlt.ShouldAvoid("deploy-1") {
		t.Error("should not avoid with healthy limits")
	}

	// Low remaining requests
	rlt.Update("deploy-2", &RateLimitInfo{
		RemainingRequests: 5,
		LimitRequests:     100,
		RemainingTokens:   5000,
		LimitTokens:       10000,
	})
	if !rlt.ShouldAvoid("deploy-2") {
		t.Error("should avoid with low remaining requests")
	}

	// Low remaining tokens
	rlt.Update("deploy-3", &RateLimitInfo{
		RemainingRequests: 50,
		LimitRequests:     100,
		RemainingTokens:   500,
		LimitTokens:       10000,
	})
	if !rlt.ShouldAvoid("deploy-3") {
		t.Error("should avoid with low remaining tokens")
	}
}

func TestRateLimitTracker_NilUpdate(t *testing.T) {
	rlt := NewRateLimitTracker()
	rlt.Update("deploy-1", nil) // should not panic
	if rlt.ShouldAvoid("deploy-1") {
		t.Error("nil update should not cause avoidance")
	}
}

func TestRouter_AddAndGetModelGroup(t *testing.T) {
	r := NewRouter(NewRegistry())
	group := &ModelGroup{
		Name:     "test-group",
		Strategy: "fallback",
		Deployments: []Deployment{
			{ID: "d1", Provider: "openai", Model: "gpt-4o", Priority: 1},
		},
	}

	r.AddModelGroup(group)

	got, err := r.GetModelGroup("test-group")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "test-group" {
		t.Errorf("expected test-group, got %s", got.Name)
	}
}

func TestRouter_GetModelGroup_NotFound(t *testing.T) {
	r := NewRouter(NewRegistry())
	_, err := r.GetModelGroup("nonexistent")
	if err == nil {
		t.Error("expected error for unknown group")
	}
}

func TestIsRetryableStatus(t *testing.T) {
	retryable := []int{429, 500, 503, 529}
	for _, status := range retryable {
		if !isRetryableStatus(status) {
			t.Errorf("expected %d to be retryable", status)
		}
	}

	nonRetryable := []int{200, 400, 401, 403, 404}
	for _, status := range nonRetryable {
		if isRetryableStatus(status) {
			t.Errorf("expected %d to NOT be retryable", status)
		}
	}
}
