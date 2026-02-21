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
	"github.com/xiaoboyu/tokengate/api-server/internal/services"
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

type mockFingerprintService struct {
	// tenantByFP maps fingerprint → tenantID (0 = not found)
	tenantByFP map[string]uint
}

func (m *mockFingerprintService) UpsertFingerprint(_ context.Context, _ string, _ uint) error {
	return nil
}

func (m *mockFingerprintService) LookupTenantByFingerprint(_ context.Context, fp string) (uint, bool, error) {
	if id, ok := m.tenantByFP[fp]; ok {
		return id, true, nil
	}
	return 0, false, nil
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func newTenantAuthRouter(apiSvc middleware.APIKeyValidatorForTest, fpSvc middleware.FingerprintStoreForTest) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/v1/messages", middleware.TenantAuthMiddlewareForTest(apiSvc, fpSvc), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"tenant_id": c.GetUint("tenant_id")})
	})
	return r
}

func doReq(r *gin.Engine, tgKey, apiKey string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	if tgKey != "" {
		req.Header.Set("X-TokenGate-Key", tgKey)
	}
	if apiKey != "" {
		req.Header.Set("X-Api-Key", apiKey)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestTenantAuth_NoHeaders_Returns401(t *testing.T) {
	r := newTenantAuthRouter(&mockAPIKeyService{}, &mockFingerprintService{})
	w := doReq(r, "", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d: %s", w.Code, w.Body)
	}
	if !strings.Contains(w.Body.String(), "tg_auth_missing") {
		t.Errorf("want tg_auth_missing in body, got: %s", w.Body)
	}
}

func TestTenantAuth_ValidTGKey_Returns200(t *testing.T) {
	apiSvc := &mockAPIKeyService{key: &models.APIKey{TenantID: 42, KeyID: "tg_abc"}}
	r := newTenantAuthRouter(apiSvc, &mockFingerprintService{})
	w := doReq(r, "tg_abc:secret", "")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}
	if !strings.Contains(w.Body.String(), "42") {
		t.Errorf("want tenant_id=42 in body, got: %s", w.Body)
	}
}

func TestTenantAuth_InvalidTGKey_Returns401(t *testing.T) {
	apiSvc := &mockAPIKeyService{err: errors.New("not found")}
	r := newTenantAuthRouter(apiSvc, &mockFingerprintService{})
	w := doReq(r, "tg_bad:secret", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d: %s", w.Code, w.Body)
	}
}

func TestTenantAuth_KnownAPIKeyFingerprint_Returns200(t *testing.T) {
	rawKey := "sk-ant-test-key-12345"
	fp := services.ComputeAPIKeyFingerprint(rawKey)
	fpSvc := &mockFingerprintService{tenantByFP: map[string]uint{fp: 77}}
	r := newTenantAuthRouter(&mockAPIKeyService{}, fpSvc)
	w := doReq(r, "", rawKey)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}
	if !strings.Contains(w.Body.String(), "77") {
		t.Errorf("want tenant_id=77 in body, got: %s", w.Body)
	}
}

func TestTenantAuth_UnknownAPIKeyFingerprint_Returns401(t *testing.T) {
	r := newTenantAuthRouter(&mockAPIKeyService{}, &mockFingerprintService{})
	w := doReq(r, "", "sk-ant-unknown-key")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d: %s", w.Code, w.Body)
	}
	if !strings.Contains(w.Body.String(), "tg_auth_missing") {
		t.Errorf("want tg_auth_missing in body, got: %s", w.Body)
	}
}

func TestTenantAuth_BothHeaders_RegistersFingerprint(t *testing.T) {
	rawKey := "sk-ant-both-headers"
	apiSvc := &mockAPIKeyService{key: &models.APIKey{TenantID: 55, KeyID: "tg_xyz"}}
	fp := services.ComputeAPIKeyFingerprint(rawKey)
	fpSvc := &mockFingerprintService{tenantByFP: map[string]uint{}}
	r := newTenantAuthRouter(apiSvc, fpSvc)

	// First request: both headers present → should register fingerprint and return 200
	w := doReq(r, "tg_xyz:secret", rawKey)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}

	// Simulate fingerprint having been registered, then re-lookup works
	fpSvc.tenantByFP[fp] = 55
	r2 := newTenantAuthRouter(&mockAPIKeyService{}, fpSvc)
	w2 := doReq(r2, "", rawKey)
	if w2.Code != http.StatusOK {
		t.Fatalf("want 200 on fingerprint-only request, got %d: %s", w2.Code, w2.Body)
	}
}
