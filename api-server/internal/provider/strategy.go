package provider

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"sync/atomic"
)

// RoutingStrategy selects a deployment from a model group.
type RoutingStrategy interface {
	Select(ctx context.Context, group *ModelGroup, state *RouterState) (*Deployment, error)
}

// RouterState provides shared state to routing strategies.
type RouterState struct {
	Health    *HealthTracker
	Latency   *LatencyTracker
	RateLimit *RateLimitTracker
	Counter   atomic.Uint64 // for round-robin
}

// NewRouterState creates a new RouterState with initialized trackers.
func NewRouterState() *RouterState {
	return &RouterState{
		Health:    NewHealthTracker(),
		Latency:   NewLatencyTracker(),
		RateLimit: NewRateLimitTracker(),
	}
}

// healthyDeployments filters deployments to only those that are healthy and not rate-limited.
func healthyDeployments(deployments []Deployment, state *RouterState) []Deployment {
	var healthy []Deployment
	for _, d := range deployments {
		if state.Health.IsHealthy(d.ID) && !state.RateLimit.ShouldAvoid(d.ID) {
			healthy = append(healthy, d)
		}
	}
	return healthy
}

// --- Fallback Strategy ---

type FallbackStrategy struct{}

func (s *FallbackStrategy) Select(_ context.Context, group *ModelGroup, state *RouterState) (*Deployment, error) {
	if len(group.Deployments) == 0 {
		return nil, fmt.Errorf("no deployments in group %s", group.Name)
	}

	// Sort by priority (lower = higher priority)
	sorted := make([]Deployment, len(group.Deployments))
	copy(sorted, group.Deployments)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Priority < sorted[j].Priority
	})

	// Try healthy ones first in priority order
	for i := range sorted {
		if state.Health.IsHealthy(sorted[i].ID) && !state.RateLimit.ShouldAvoid(sorted[i].ID) {
			return &sorted[i], nil
		}
	}

	// All in cooldown — try the one that recovers soonest
	ids := make([]string, len(sorted))
	for i, d := range sorted {
		ids[i] = d.ID
	}
	earliest := state.Health.EarliestCooldownExpiry(ids)
	for i := range sorted {
		if sorted[i].ID == earliest {
			return &sorted[i], nil
		}
	}

	return &sorted[0], nil
}

// --- Round-Robin Strategy ---

type RoundRobinStrategy struct{}

func (s *RoundRobinStrategy) Select(_ context.Context, group *ModelGroup, state *RouterState) (*Deployment, error) {
	if len(group.Deployments) == 0 {
		return nil, fmt.Errorf("no deployments in group %s", group.Name)
	}

	healthy := healthyDeployments(group.Deployments, state)
	if len(healthy) == 0 {
		// Fall back to all deployments
		healthy = group.Deployments
	}

	// Check if any deployment has weight > 0 for weighted selection
	hasWeights := false
	totalWeight := 0
	for _, d := range healthy {
		if d.Weight > 0 {
			hasWeights = true
			totalWeight += d.Weight
		}
	}

	if hasWeights && totalWeight > 0 {
		return weightedSelect(healthy, totalWeight, state)
	}

	// Simple round-robin
	idx := state.Counter.Add(1) - 1
	return &healthy[idx%uint64(len(healthy))], nil
}

func weightedSelect(deployments []Deployment, totalWeight int, state *RouterState) (*Deployment, error) {
	idx := int(state.Counter.Add(1)-1) % totalWeight
	cumulative := 0
	for i := range deployments {
		w := deployments[i].Weight
		if w <= 0 {
			w = 1
		}
		cumulative += w
		if idx < cumulative {
			return &deployments[i], nil
		}
	}
	return &deployments[0], nil
}

// --- Lowest Latency Strategy ---

type LowestLatencyStrategy struct {
	explorationRate float64 // fraction of requests to explore (default 0.1)
}

func NewLowestLatencyStrategy() *LowestLatencyStrategy {
	return &LowestLatencyStrategy{explorationRate: 0.1}
}

func (s *LowestLatencyStrategy) Select(_ context.Context, group *ModelGroup, state *RouterState) (*Deployment, error) {
	if len(group.Deployments) == 0 {
		return nil, fmt.Errorf("no deployments in group %s", group.Name)
	}

	healthy := healthyDeployments(group.Deployments, state)
	if len(healthy) == 0 {
		healthy = group.Deployments
	}

	// Find deployments without latency data (need exploration)
	var unexplored []Deployment
	var explored []Deployment
	for _, d := range healthy {
		if state.Latency.HasData(d.ID) {
			explored = append(explored, d)
		} else {
			unexplored = append(unexplored, d)
		}
	}

	// Exploration: randomly pick unexplored deployment
	if len(unexplored) > 0 && rand.Float64() < s.explorationRate {
		idx := rand.Intn(len(unexplored))
		return &unexplored[idx], nil
	}

	// If no explored deployments, pick randomly
	if len(explored) == 0 {
		idx := rand.Intn(len(healthy))
		return &healthy[idx], nil
	}

	// Pick the one with lowest average latency
	var best *Deployment
	var bestLatency = state.Latency.Average(explored[0].ID)
	best = &explored[0]

	for i := 1; i < len(explored); i++ {
		avg := state.Latency.Average(explored[i].ID)
		if avg < bestLatency {
			bestLatency = avg
			best = &explored[i]
		}
	}
	return best, nil
}

// --- Cost-Optimized Strategy ---

type CostOptimizedStrategy struct{}

func (s *CostOptimizedStrategy) Select(_ context.Context, group *ModelGroup, state *RouterState) (*Deployment, error) {
	if len(group.Deployments) == 0 {
		return nil, fmt.Errorf("no deployments in group %s", group.Name)
	}

	healthy := healthyDeployments(group.Deployments, state)
	if len(healthy) == 0 {
		healthy = group.Deployments
	}

	// Sort by total cost (input + output), pick cheapest
	sort.Slice(healthy, func(i, j int) bool {
		costI := healthy[i].CostPer1KInput + healthy[i].CostPer1KOutput
		costJ := healthy[j].CostPer1KInput + healthy[j].CostPer1KOutput
		return costI < costJ
	})

	return &healthy[0], nil
}

// GetStrategy returns a RoutingStrategy for the given strategy name.
func GetStrategy(name string) (RoutingStrategy, error) {
	switch name {
	case "fallback":
		return &FallbackStrategy{}, nil
	case "round-robin":
		return &RoundRobinStrategy{}, nil
	case "lowest-latency":
		return NewLowestLatencyStrategy(), nil
	case "cost-optimized":
		return &CostOptimizedStrategy{}, nil
	default:
		return nil, fmt.Errorf("unknown routing strategy: %s", name)
	}
}
