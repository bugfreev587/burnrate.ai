package middleware

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/xiaoboyu/tokengate/api-server/internal/services"
)

const (
	ContextKeyTenantSlug   = "tenant_slug"
	ErrCodeTenantMissing   = "tg_tenant_missing"
	ErrCodeInvalidSlug     = "tg_invalid_slug"
	ErrCodeTenantNotFound  = "tg_tenant_not_found"
	ErrCodeTenantSuspended = "tg_tenant_suspended"
)

// slugRE: 3–40 chars; starts and ends with alphanumeric, may contain hyphens in between.
var slugRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{1,38}[a-z0-9])?$`)

// TenantPathMiddleware identifies the tenant from the :tenant_slug route parameter.
// It validates the slug format, resolves the tenant via TenantLookup, sets
// "tenant_id" (uint) and "tenant_slug" (string) in the gin context, and rewrites
// c.Request.URL.Path by stripping the leading /{slug} prefix so that downstream
// proxy handlers receive a /v1/… path unchanged.
func TenantPathMiddleware(svc services.TenantLookup) gin.HandlerFunc {
	return func(c *gin.Context) {
		slug := c.Param("tenant_slug")
		if slug == "" || !slugRE.MatchString(slug) {
			c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
				"type":    ErrCodeInvalidSlug,
				"message": "Invalid tenant slug. Use /{tenant_slug}/v1/... with a lowercase alphanumeric slug (3–40 chars).",
			}})
			c.Abort()
			return
		}

		tenant, err := svc.GetTenantBySlug(c.Request.Context(), slug)
		if err != nil {
			switch err {
			case services.ErrTenantNotFound:
				c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{
					"type":    ErrCodeTenantNotFound,
					"message": "Unknown tenant.",
				}})
			case services.ErrTenantSuspended:
				c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{
					"type":    ErrCodeTenantSuspended,
					"message": "Tenant account is suspended.",
				}})
			default:
				c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
					"type":    "tg_internal_error",
					"message": "Failed to resolve tenant.",
				}})
			}
			c.Abort()
			return
		}

		c.Set("tenant_id", tenant.ID)
		c.Set(ContextKeyTenantSlug, tenant.Slug)

		// Rewrite: /acme/v1/messages → /v1/messages
		// Gin selects the handler before middleware runs, so this only affects
		// proxy handlers that inspect c.Request.URL.Path directly (e.g. resolveProvider).
		stripped := strings.TrimPrefix(c.Request.URL.Path, "/"+slug)
		if stripped == "" {
			stripped = "/"
		}
		c.Request.URL.Path = stripped

		c.Next()
	}
}
