package middleware

import (
	"context"
	"fmt"
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
// When the EnableGatewayValidate env var is set to "false", all requests are
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
		fmt.Println("------- TenantAuthMiddleware -------")
		fmt.Println("------- EnableGatewayValidate:", os.Getenv("EnableGatewayValidate"))
		// When gateway validation is disabled, forward all requests as-is.
		if strings.EqualFold(os.Getenv("EnableGatewayValidate"), "false") {
			c.Next()
			return
		}

		tgKey := strings.TrimSpace(c.GetHeader("X-TokenGate-Key"))
		fmt.Println("------- X-TokenGate-Key:", tgKey)
		if tgKey == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{
				"type":    ErrCodeMissingKey,
				"message": "Authentication required. Provide the X-TokenGate-Key header.",
			}})
			c.Abort()
			return
		}

		ak, err := apiKeySvc.ValidateKey(c.Request.Context(), tgKey)
		fmt.Println("------ validation api-key ak:", ak, "err:", err)
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
		fmt.Println("----- validation passed, setting context and proceeding ----- tenantID: ", ak.TenantID, "keyID:", ak.KeyID)

		c.Set(ContextKeyAPIKey, ak)
		c.Set("tenant_id", ak.TenantID)
		c.Set("key_id", ak.KeyID)
		go apiKeySvc.TouchLastSeen(context.Background(), ak.KeyID)
		c.Next()
	}
}
