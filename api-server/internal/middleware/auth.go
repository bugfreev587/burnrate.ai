package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/xiaoboyu/tokengate/api-server/internal/services"
)

const (
	ErrCodeMissingKey = "tg_auth_missing"
	ErrCodeInvalidKey = "tg_auth_invalid"
	ErrCodeExpiredKey = "tg_auth_expired"
	ErrCodeRevokedKey = "tg_auth_revoked"

	ContextKeyAPIKey = "api_key"
)

// APIKeyMiddleware validates the API key from the X-TokenGate-Key header.
func APIKeyMiddleware(svc *services.APIKeyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		fmt.Println("------- APIKeyMiddleware -------")
		fmt.Println("---------- Headers:", c.Request.Header)
		fmt.Println("---------- X-TokenGate-Key:", c.GetHeader("X-TokenGate-Key"))
		token := strings.TrimSpace(c.GetHeader("X-TokenGate-Key"))
		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{
				"type":    ErrCodeMissingKey,
				"message": "X-TokenGate-Key header is required.",
			}})
			c.Abort()
			return
		}

		ak, err := svc.ValidateKey(c.Request.Context(), token)
		fmt.Println("------- validation api-key ak:", ak, "err:", err)
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
		c.Set("key_id", ak.KeyID)
		c.Set("provider", ak.Provider)
		c.Set("mode", ak.Mode)
		go svc.TouchLastSeen(context.Background(), ak.KeyID)
		c.Next()
	}
}
