package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/xiaoboyu/tokengate/api-server/internal/models"
	"github.com/xiaoboyu/tokengate/api-server/internal/services"
)

// APIKeyValidatorForTest is the subset of APIKeyService used by TenantAuthMiddleware.
// Exposed so test packages can provide mock implementations.
type APIKeyValidatorForTest interface {
	ValidateKey(ctx context.Context, presented string) (*models.APIKey, error)
	TouchLastSeen(ctx context.Context, keyID string)
}

// TenantAuthMiddleware validates requests using the X-TokenGate-Key header.
//
// When the ENABLE_GW_VALIDATION env var is set to "false", all requests are
// passed through to the upstream without validation.
// Otherwise, X-TokenGate-Key must be present and valid; requests that fail
// validation are rejected with 401.
func TenantAuthMiddleware(apiKeySvc *services.APIKeyService) gin.HandlerFunc {
	return TenantAuthMiddlewareForTest(apiKeySvc)
}

// TenantAuthMiddlewareForTest is the testable variant that accepts the interface type.
// Production code should use TenantAuthMiddleware.
func TenantAuthMiddlewareForTest(apiKeySvc APIKeyValidatorForTest) gin.HandlerFunc {
	return func(c *gin.Context) {
		// When gateway validation is disabled, forward all requests as-is.
		if strings.EqualFold(os.Getenv("ENABLE_GW_VALIDATION"), "false") {
			c.Next()
			return
		}

		tgKey := strings.TrimSpace(c.GetHeader("X-TokenGate-Key"))
		slog.Info("X-TokenGate-Key in tenant auth middle", "key", tgKey)
		// Fallback: accept Authorization: Bearer <key> (used by OpenAI-compatible clients like Codex CLI).
		if tgKey == "" {
			if auth := c.GetHeader("Authorization"); strings.HasPrefix(auth, "Bearer ") {
				tgKey = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
			}
		}
		if tgKey == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{
				"type":    ErrCodeMissingKey,
				"message": "Authentication required. Provide the X-TokenGate-Key or Authorization: Bearer header.",
			}})
			c.Abort()
			return
		}
		ak, err := apiKeySvc.ValidateKey(c.Request.Context(), tgKey)
		if err != nil {
			errStr := err.Error()
			code := ErrCodeInvalidKey
			msg := "Invalid API key."
			switch {
			case strings.Contains(errStr, "expired"):
				code = ErrCodeExpiredKey
				msg = "API key has expired."
			case strings.Contains(errStr, "revoked"):
				code = ErrCodeRevokedKey
				msg = "API key has been revoked."
			case strings.Contains(errStr, "not found"):
				msg = "API key not found."
			}
			c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"type": code, "message": msg}})
			c.Abort()
			return
		}
		c.Set(ContextKeyAPIKey, ak)
		c.Set("tenant_id", ak.TenantID)
		c.Set("project_id", ak.ProjectID)
		c.Set("key_id", ak.KeyID)
		c.Set("provider", ak.Provider)
		c.Set("auth_method", ak.AuthMethod)
		c.Set("billing_mode", ak.BillingMode)
		go apiKeySvc.TouchLastSeen(context.Background(), ak.KeyID)
		c.Next()
	}
}
