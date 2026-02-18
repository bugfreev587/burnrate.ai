package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/xiaoboyu/burnrate-ai/api-server/internal/services"
)

const (
	ErrCodeMissingKey = "api_key_missing"
	ErrCodeInvalidKey = "api_key_invalid"
	ErrCodeExpiredKey = "api_key_expired"
	ErrCodeRevokedKey = "api_key_revoked"
	ErrCodeBadFormat  = "api_key_bad_format"

	ContextKeyAPIKey = "api_key"
)

// APIKeyMiddleware validates the API key from Authorization or X-Api-Key header.
func APIKeyMiddleware(svc *services.APIKeyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth == "" {
			auth = c.GetHeader("X-Api-Key")
		}
		if auth == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   ErrCodeMissingKey,
				"message": "API key required. Use 'Authorization: ApiKey <key>' or 'X-Api-Key' header.",
			})
			c.Abort()
			return
		}

		token := strings.TrimSpace(strings.TrimPrefix(auth, "ApiKey"))

		ak, err := svc.ValidateKey(c.Request.Context(), token)
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
			case strings.Contains(errStr, "bad key format"):
				code = ErrCodeBadFormat
				msg = "Invalid API key format. Expected 'keyid:secret'."
			case strings.Contains(errStr, "not found"):
				msg = "API key not found."
			}
			c.JSON(http.StatusUnauthorized, gin.H{"error": code, "message": msg})
			c.Abort()
			return
		}

		c.Set(ContextKeyAPIKey, ak)
		c.Set("tenant_id", ak.TenantID)
		c.Set("key_id", ak.KeyID)
		c.Next()
	}
}
