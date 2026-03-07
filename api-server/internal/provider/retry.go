package provider

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// RetryConfig configures the retry behavior for routed requests.
type RetryConfig struct {
	MaxRetries         int           // max retry attempts per user request (default 3)
	ConnectTimeout     time.Duration // TCP connect timeout (default 5s)
	ReadTimeout        time.Duration // read timeout for non-streaming (default 60s)
	StreamFirstChunkTO time.Duration // timeout for first streaming chunk (default 30s)
	BaseBackoff        time.Duration // base backoff for 500/503 errors (default 1s)
	MaxBackoff         time.Duration // max backoff duration (default 8s)
}

// DefaultRetryConfig returns sensible defaults for production use.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:         3,
		ConnectTimeout:     5 * time.Second,
		ReadTimeout:        60 * time.Second,
		StreamFirstChunkTO: 30 * time.Second,
		BaseBackoff:        1 * time.Second,
		MaxBackoff:         8 * time.Second,
	}
}

// RetryExecutor handles request execution with retries across deployments.
type RetryExecutor struct {
	config   RetryConfig
	registry *Registry
	state    *RouterState
}

func NewRetryExecutor(config RetryConfig, registry *Registry, state *RouterState) *RetryExecutor {
	return &RetryExecutor{
		config:   config,
		registry: registry,
		state:    state,
	}
}

// Attempt represents the result of a single execution attempt.
type Attempt struct {
	Deployment *Deployment
	Response   *ChatCompletionResponse
	Error      *ProviderError
	RawBody    []byte
	StatusCode int
	Duration   time.Duration
	TTFB       time.Duration // time to first byte
}

// ExecuteWithRetry runs a request against a model group with retry logic.
// It uses the group's routing strategy for deployment selection and retries
// on retryable errors with appropriate backoff.
func (re *RetryExecutor) ExecuteWithRetry(
	ctx context.Context,
	group *ModelGroup,
	req *ChatCompletionRequest,
) (*Attempt, []Attempt, error) {
	strategy, err := GetStrategy(group.Strategy)
	if err != nil {
		return nil, nil, err
	}

	maxAttempts := re.config.MaxRetries + 1
	if maxAttempts > len(group.Deployments) {
		maxAttempts = len(group.Deployments)
	}

	var attempts []Attempt
	tried := make(map[string]bool)

	for i := 0; i < maxAttempts; i++ {
		if ctx.Err() != nil {
			return nil, attempts, ctx.Err()
		}

		deployment, err := strategy.Select(ctx, group, re.state)
		if err != nil {
			return nil, attempts, fmt.Errorf("select deployment: %w", err)
		}
		if tried[deployment.ID] {
			continue
		}
		tried[deployment.ID] = true

		adapter, err := re.registry.Get(deployment.Provider)
		if err != nil {
			continue
		}

		attempt := re.executeOnce(ctx, req, deployment, adapter)
		attempts = append(attempts, attempt)

		if attempt.Error == nil {
			re.state.Health.RecordSuccess(deployment.ID)
			return &attempt, attempts, nil
		}

		re.state.Health.RecordFailure(deployment.ID)
		slog.Warn("provider attempt failed",
			"provider", deployment.Provider,
			"model", deployment.Model,
			"deployment", deployment.ID,
			"status", attempt.StatusCode,
			"error", attempt.Error.Message,
			"attempt", i+1,
		)

		if !attempt.Error.Retryable {
			return &attempt, attempts, attempt.Error
		}

		// Backoff before retry
		backoff := re.computeBackoff(i, &attempt)
		if backoff > 0 {
			select {
			case <-ctx.Done():
				return nil, attempts, ctx.Err()
			case <-time.After(backoff):
			}
		}
	}

	if len(attempts) == 0 {
		return nil, nil, fmt.Errorf("no deployments available in group %s", group.Name)
	}

	lastAttempt := attempts[len(attempts)-1]
	return &lastAttempt, attempts, fmt.Errorf("all %d deployments exhausted", len(attempts))
}

func (re *RetryExecutor) executeOnce(
	ctx context.Context,
	req *ChatCompletionRequest,
	dep *Deployment,
	adapter ProviderAdapter,
) Attempt {
	start := time.Now()

	reqCopy := *req
	reqCopy.Model = dep.Model

	httpReq, err := adapter.TransformRequest(ctx, &reqCopy, dep.APIKey)
	if err != nil {
		return Attempt{
			Deployment: dep,
			Error: &ProviderError{
				StatusCode: 0,
				Message:    fmt.Sprintf("transform request: %v", err),
				Type:       ErrorTypeServer,
				Provider:   dep.Provider,
				Model:      dep.Model,
				Retryable:  false,
			},
			Duration: time.Since(start),
		}
	}

	// Build client with appropriate timeouts
	transport := &http.Transport{
		ResponseHeaderTimeout: re.config.ConnectTimeout + re.config.ReadTimeout,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   re.config.ConnectTimeout + re.config.ReadTimeout,
	}
	if req.Stream {
		// For streaming, only enforce connect + first-chunk timeout
		client.Timeout = 0 // no overall timeout for streaming
		transport.ResponseHeaderTimeout = re.config.StreamFirstChunkTO
	}

	resp, err := client.Do(httpReq)
	ttfb := time.Since(start)

	if err != nil {
		errType := ErrorTypeServer
		retryable := true
		if ctx.Err() != nil {
			errType = ErrorTypeTimeout
		}
		return Attempt{
			Deployment: dep,
			Error: &ProviderError{
				StatusCode: 0,
				Message:    fmt.Sprintf("request failed: %v", err),
				Type:       errType,
				Provider:   dep.Provider,
				Model:      dep.Model,
				Retryable:  retryable,
			},
			Duration: time.Since(start),
			TTFB:     ttfb,
		}
	}

	re.state.Latency.Record(dep.ID, ttfb)

	// Extract rate limit info from headers
	rlInfo := adapter.ExtractRateLimitInfo(resp.Header)
	re.state.RateLimit.Update(dep.ID, rlInfo)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		pe := ParseProviderError(resp.StatusCode, body, dep.Provider, dep.Model, resp.Header)
		return Attempt{
			Deployment: dep,
			Error:      pe,
			RawBody:    body,
			StatusCode: resp.StatusCode,
			Duration:   time.Since(start),
			TTFB:       ttfb,
		}
	}

	chatResp, err := adapter.TransformResponse(resp)
	if err != nil {
		return Attempt{
			Deployment: dep,
			Error: &ProviderError{
				StatusCode: 200,
				Message:    fmt.Sprintf("transform response: %v", err),
				Type:       ErrorTypeServer,
				Provider:   dep.Provider,
				Model:      dep.Model,
				Retryable:  false,
			},
			Duration: time.Since(start),
			TTFB:     ttfb,
		}
	}

	return Attempt{
		Deployment: dep,
		Response:   chatResp,
		StatusCode: http.StatusOK,
		Duration:   time.Since(start),
		TTFB:       ttfb,
	}
}

func (re *RetryExecutor) computeBackoff(attemptNum int, attempt *Attempt) time.Duration {
	if attempt.Error == nil {
		return 0
	}

	// Timeout → immediate retry on next deployment
	if attempt.Error.Type == ErrorTypeTimeout {
		return 0
	}

	// 429 → use Retry-After if available, else default 1s
	if attempt.StatusCode == http.StatusTooManyRequests {
		if attempt.Error.RetryAfter > 0 {
			d := time.Duration(attempt.Error.RetryAfter) * time.Second
			if d > re.config.MaxBackoff {
				d = re.config.MaxBackoff
			}
			return d
		}
		return re.config.BaseBackoff
	}

	// 500/503 → exponential backoff
	backoff := re.config.BaseBackoff * (1 << attemptNum)
	if backoff > re.config.MaxBackoff {
		backoff = re.config.MaxBackoff
	}
	return backoff
}
