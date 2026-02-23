package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/xiaoboyu/tokengate/api-server/internal/middleware"
	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

// ── Mocks ────────────────────────────────────────────────────────────────────

type mockAPIKeyService struct {
	key *models.APIKey
	err error
}

func (m *mockAPIKeyService) ValidateKey(_ context.Context, _ string) (*models.APIKey, error) {
	return m.key, m.err
}

func (m *mockAPIKeyService) TouchLastSeen(_ context.Context, _ string) {}

// ── Helpers ──────────────────────────────────────────────────────────────────

func newTenantAuthRouter(apiSvc middleware.APIKeyValidatorForTest) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/v1/messages", middleware.TenantAuthMiddlewareForTest(apiSvc), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"tenant_id": c.GetUint("tenant_id")})
	})
	return r
}

func doReq(r *gin.Engine, tgKey string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	if tgKey != "" {
		req.Header.Set("X-TokenGate-Key", tgKey)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestTenantAuth_NoHeader_Returns401(t *testing.T) {
	r := newTenantAuthRouter(&mockAPIKeyService{})
	w := doReq(r, "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d: %s", w.Code, w.Body)
	}
	if !strings.Contains(w.Body.String(), "tg_auth_missing") {
		t.Errorf("want tg_auth_missing in body, got: %s", w.Body)
	}
}

func TestTenantAuth_ValidTGKey_Returns200(t *testing.T) {
	apiSvc := &mockAPIKeyService{key: &models.APIKey{TenantID: 42, KeyID: "tg_abc"}}
	r := newTenantAuthRouter(apiSvc)
	w := doReq(r, "tg_abc:secret")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}
	if !strings.Contains(w.Body.String(), "42") {
		t.Errorf("want tenant_id=42 in body, got: %s", w.Body)
	}
}

func TestTenantAuth_InvalidTGKey_Returns401(t *testing.T) {
	apiSvc := &mockAPIKeyService{err: errors.New("not found")}
	r := newTenantAuthRouter(apiSvc)
	w := doReq(r, "tg_bad:secret")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d: %s", w.Code, w.Body)
	}
}

func TestTenantAuth_GatewayValidateDisabled_PassesThrough(t *testing.T) {
	t.Setenv("ENABLE_GW_VALIDATION", "false")
	// No valid key configured — still expect 200 because validation is disabled.
	r := newTenantAuthRouter(&mockAPIKeyService{err: errors.New("should not be called")})
	w := doReq(r, "") // no X-TokenGate-Key header
	if w.Code != http.StatusOK {
		t.Fatalf("want 200 (pass-through), got %d: %s", w.Code, w.Body)
	}
}
