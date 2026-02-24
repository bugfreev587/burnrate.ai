package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/xiaoboyu/tokengate/api-server/internal/middleware"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// setupGuardRouter creates a router with PathProviderGuard for a given path.
// The handler sets the provider in context (simulating tenant auth), then runs the guard.
func setupGuardRouter(path, method string) *gin.Engine {
	r := gin.New()
	// The "provider" context key is set by the test via a preceding middleware.
	switch method {
	case http.MethodGet:
		r.GET(path, middleware.PathProviderGuard(), okHandler)
	default:
		r.POST(path, middleware.PathProviderGuard(), okHandler)
	}
	return r
}

func okHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// setProvider returns a middleware that sets the "provider" context key.
func setProvider(provider string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if provider != "" {
			c.Set("provider", provider)
		}
		c.Next()
	}
}

func setupGuardRouterWithProvider(path, method, provider string) *gin.Engine {
	r := gin.New()
	switch method {
	case http.MethodGet:
		r.GET(path, setProvider(provider), middleware.PathProviderGuard(), okHandler)
	default:
		r.POST(path, setProvider(provider), middleware.PathProviderGuard(), okHandler)
	}
	return r
}

func doGuardReq(r *gin.Engine, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// ── /v1/messages — Anthropic only ──────────────────────────────────────────

func TestPathProvider_Messages_AnthropicKey_Allowed(t *testing.T) {
	r := setupGuardRouterWithProvider("/v1/messages", http.MethodPost, "anthropic")
	w := doGuardReq(r, http.MethodPost, "/v1/messages")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}
}

func TestPathProvider_Messages_OpenAIKey_Rejected(t *testing.T) {
	r := setupGuardRouterWithProvider("/v1/messages", http.MethodPost, "openai")
	w := doGuardReq(r, http.MethodPost, "/v1/messages")
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d: %s", w.Code, w.Body)
	}
}

// ── /v1/models — Anthropic only ────────────────────────────────────────────

func TestPathProvider_Models_AnthropicKey_Allowed(t *testing.T) {
	r := setupGuardRouterWithProvider("/v1/models", http.MethodGet, "anthropic")
	w := doGuardReq(r, http.MethodGet, "/v1/models")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}
}

func TestPathProvider_Models_OpenAIKey_Rejected(t *testing.T) {
	r := setupGuardRouterWithProvider("/v1/models", http.MethodGet, "openai")
	w := doGuardReq(r, http.MethodGet, "/v1/models")
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d: %s", w.Code, w.Body)
	}
}

// ── /v1/responses — Anthropic + OpenAI ─────────────────────────────────────

func TestPathProvider_Responses_AnthropicKey_Allowed(t *testing.T) {
	r := setupGuardRouterWithProvider("/v1/responses", http.MethodPost, "anthropic")
	w := doGuardReq(r, http.MethodPost, "/v1/responses")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}
}

func TestPathProvider_Responses_OpenAIKey_Allowed(t *testing.T) {
	r := setupGuardRouterWithProvider("/v1/responses", http.MethodPost, "openai")
	w := doGuardReq(r, http.MethodPost, "/v1/responses")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}
}

func TestPathProvider_Responses_GeminiKey_Rejected(t *testing.T) {
	r := setupGuardRouterWithProvider("/v1/responses", http.MethodPost, "gemini")
	w := doGuardReq(r, http.MethodPost, "/v1/responses")
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d: %s", w.Code, w.Body)
	}
}

// ── /v1/openai/* — OpenAI only ─────────────────────────────────────────────

func TestPathProvider_OpenAIPath_OpenAIKey_Allowed(t *testing.T) {
	r := gin.New()
	r.Any("/v1/openai/*path", setProvider("openai"), middleware.PathProviderGuard(), okHandler)
	w := doGuardReq(r, http.MethodPost, "/v1/openai/chat/completions")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}
}

func TestPathProvider_OpenAIPath_AnthropicKey_Rejected(t *testing.T) {
	r := gin.New()
	r.Any("/v1/openai/*path", setProvider("anthropic"), middleware.PathProviderGuard(), okHandler)
	w := doGuardReq(r, http.MethodPost, "/v1/openai/chat/completions")
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d: %s", w.Code, w.Body)
	}
}

// ── No provider in context (gateway validation disabled) ───────────────────

func TestPathProvider_NoProvider_Allowed(t *testing.T) {
	r := setupGuardRouterWithProvider("/v1/messages", http.MethodPost, "")
	w := doGuardReq(r, http.MethodPost, "/v1/messages")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200 (no provider = skip guard), got %d: %s", w.Code, w.Body)
	}
}
