package provider

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Router manages model group routing with health tracking, latency monitoring,
// and rate limit awareness.
type Router struct {
	mu       sync.RWMutex
	groups   map[string]*ModelGroup
	registry *Registry
	state    *RouterState
}

func NewRouter(registry *Registry) *Router {
	return &Router{
		groups:   make(map[string]*ModelGroup),
		registry: registry,
		state:    NewRouterState(),
	}
}

// AddModelGroup registers a model group for routing.
func (r *Router) AddModelGroup(group *ModelGroup) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.groups[group.Name] = group
}

// RemoveModelGroup removes a model group from the router.
func (r *Router) RemoveModelGroup(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.groups, name)
}

// HasModelGroup returns true if a model group with the given name exists.
func (r *Router) HasModelGroup(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.groups[name]
	return ok
}

// GetModelGroup returns the model group with the given name.
func (r *Router) GetModelGroup(name string) (*ModelGroup, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	g, ok := r.groups[name]
	if !ok {
		return nil, fmt.Errorf("unknown model group: %s", name)
	}
	return g, nil
}

// RouteResult contains the result of a routing + execution cycle.
type RouteResult struct {
	Response   *ChatCompletionResponse
	Deployment *Deployment
	Adapter    ProviderAdapter
	StatusCode int
	RawBody    []byte
}

// Route selects a deployment and executes the request against it. For fallback strategy,
// retries with the next deployment on retryable errors.
func (r *Router) Route(ctx context.Context, groupName string, req *ChatCompletionRequest) (*RouteResult, error) {
	group, err := r.GetModelGroup(groupName)
	if err != nil {
		return nil, err
	}

	strategy, err := GetStrategy(group.Strategy)
	if err != nil {
		return nil, err
	}

	// For fallback, we may retry multiple deployments
	maxAttempts := 1
	if group.Strategy == "fallback" {
		maxAttempts = len(group.Deployments)
	}

	var lastErr error
	tried := make(map[string]bool)

	for attempt := 0; attempt < maxAttempts; attempt++ {
		deployment, err := strategy.Select(ctx, group, r.state)
		if err != nil {
			return nil, err
		}

		if tried[deployment.ID] {
			continue
		}
		tried[deployment.ID] = true

		adapter, err := r.registry.Get(deployment.Provider)
		if err != nil {
			lastErr = err
			continue
		}

		// Override model in request to the deployment's actual model
		reqCopy := *req
		reqCopy.Model = deployment.Model

		result, err := r.executeRequest(ctx, &reqCopy, deployment, adapter)
		if err != nil {
			r.state.Health.RecordFailure(deployment.ID)
			lastErr = err
			if group.Strategy == "fallback" {
				continue
			}
			return nil, err
		}

		// Check for retryable status codes
		if isRetryableStatus(result.StatusCode) {
			r.state.Health.RecordFailure(deployment.ID)
			lastErr = fmt.Errorf("provider returned status %d", result.StatusCode)
			if group.Strategy == "fallback" {
				continue
			}
			return nil, lastErr
		}

		r.state.Health.RecordSuccess(deployment.ID)
		return result, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("all deployments exhausted: %w", lastErr)
	}
	return nil, fmt.Errorf("no deployments available in group %s", groupName)
}

func (r *Router) executeRequest(ctx context.Context, req *ChatCompletionRequest, dep *Deployment, adapter ProviderAdapter) (*RouteResult, error) {
	start := time.Now()

	httpReq, err := adapter.TransformRequest(ctx, req, dep.APIKey)
	if err != nil {
		return nil, fmt.Errorf("transform request: %w", err)
	}

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	latency := time.Since(start)
	r.state.Latency.Record(dep.ID, latency)

	// Extract and store rate limit info
	rlInfo := adapter.ExtractRateLimitInfo(resp.Header)
	r.state.RateLimit.Update(dep.ID, rlInfo)

	if resp.StatusCode != http.StatusOK {
		body, _ := readBody(resp)
		return &RouteResult{
			Deployment: dep,
			Adapter:    adapter,
			StatusCode: resp.StatusCode,
			RawBody:    body,
		}, nil
	}

	// Transform the response
	chatResp, err := adapter.TransformResponse(resp)
	if err != nil {
		return nil, fmt.Errorf("transform response: %w", err)
	}

	return &RouteResult{
		Response:   chatResp,
		Deployment: dep,
		Adapter:    adapter,
		StatusCode: http.StatusOK,
	}, nil
}

func readBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	var buf [64 * 1024]byte
	var body []byte
	for {
		n, err := resp.Body.Read(buf[:])
		if n > 0 {
			body = append(body, buf[:n]...)
		}
		if err != nil {
			break
		}
	}
	return body, nil
}

func isRetryableStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests, // 429
		http.StatusInternalServerError,    // 500
		http.StatusServiceUnavailable,     // 503
		529:                               // Anthropic overloaded
		return true
	default:
		return false
	}
}

// State returns the router's shared state for external inspection or testing.
func (r *Router) State() *RouterState {
	return r.state
}
