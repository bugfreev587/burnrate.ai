package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/xiaoboyu/tokengate/api-server/internal/middleware"
	"github.com/xiaoboyu/tokengate/api-server/internal/models"
	"github.com/xiaoboyu/tokengate/api-server/internal/services"
)

type mockTenantLookup struct {
	tenants map[string]*models.Tenant
}

func (m *mockTenantLookup) GetTenantBySlug(_ context.Context, slug string) (*models.Tenant, error) {
	t, ok := m.tenants[slug]
	if !ok {
		return nil, services.ErrTenantNotFound
	}
	if t.Status != models.StatusActive {
		return nil, services.ErrTenantSuspended
	}
	return t, nil
}

func newMock() *mockTenantLookup {
	return &mockTenantLookup{tenants: map[string]*models.Tenant{
		"acme":     {ID: 42, Slug: "acme", Status: models.StatusActive},
		"frozenco": {ID: 99, Slug: "frozenco", Status: models.StatusSuspended},
	}}
}

func newRouter(mock services.TenantLookup) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/:tenant_slug/v1/messages", middleware.TenantPathMiddleware(mock), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"tenant_id": c.GetUint("tenant_id")})
	})
	return r
}

func do(r *gin.Engine, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestValidPath(t *testing.T) {
	w := do(newRouter(newMock()), "/acme/v1/messages")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}
}

func TestInvalidSlugFormat(t *testing.T) {
	w := do(newRouter(newMock()), "/ACME/v1/messages")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", w.Code, w.Body)
	}
	if !strings.Contains(w.Body.String(), middleware.ErrCodeInvalidSlug) {
		t.Errorf("want %q in body, got: %s", middleware.ErrCodeInvalidSlug, w.Body)
	}
}

func TestUnknownTenant(t *testing.T) {
	w := do(newRouter(newMock()), "/nobody/v1/messages")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d: %s", w.Code, w.Body)
	}
	if !strings.Contains(w.Body.String(), middleware.ErrCodeTenantNotFound) {
		t.Errorf("want %q in body, got: %s", middleware.ErrCodeTenantNotFound, w.Body)
	}
}

func TestSuspendedTenant(t *testing.T) {
	w := do(newRouter(newMock()), "/frozenco/v1/messages")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d: %s", w.Code, w.Body)
	}
	if !strings.Contains(w.Body.String(), middleware.ErrCodeTenantSuspended) {
		t.Errorf("want %q in body, got: %s", middleware.ErrCodeTenantSuspended, w.Body)
	}
}

func TestPathRewrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	var seenPath string
	r.POST("/:tenant_slug/v1/messages", middleware.TenantPathMiddleware(newMock()), func(c *gin.Context) {
		seenPath = c.Request.URL.Path
		c.JSON(http.StatusOK, gin.H{})
	})
	w := do(r, "/acme/v1/messages")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}
	if seenPath != "/v1/messages" {
		t.Errorf("want path /v1/messages after rewrite, got: %q", seenPath)
	}
}

func TestQueryStringPreserved(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	var seenQuery string
	r.POST("/:tenant_slug/v1/messages", middleware.TenantPathMiddleware(newMock()), func(c *gin.Context) {
		seenQuery = c.Request.URL.RawQuery
		c.JSON(http.StatusOK, gin.H{})
	})
	req := httptest.NewRequest(http.MethodPost, "/acme/v1/messages?stream=true", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}
	if seenQuery != "stream=true" {
		t.Errorf("want query stream=true, got: %q", seenQuery)
	}
}
