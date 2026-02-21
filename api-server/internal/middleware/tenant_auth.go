package middleware

import (
	"context"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/xiaoboyu/tokengate/api-server/internal/models"
	"github.com/xiaoboyu/tokengate/api-server/internal/services"
)

// ContextKeyFingerprint is the Gin context key for the stable API key fingerprint
// ("ak:<sha256-hex>") derived from the X-Api-Key header.
const ContextKeyFingerprint = "api_key_fingerprint"

// APIKeyValidatorForTest is the subset of APIKeyService used by TenantAuthMiddleware.
// Exposed so test packages can provide mock implementations.
type APIKeyValidatorForTest interface {
	ValidateKey(ctx context.Context, presented string) (*models.APIKey, error)
	TouchLastSeen(ctx context.Context, keyID string)
}

// FingerprintStoreForTest is the subset of FingerprintService used by TenantAuthMiddleware.
// Exposed so test packages can provide mock implementations.
type FingerprintStoreForTest interface {
	UpsertFingerprint(ctx context.Context, fingerprint string, tenantID uint) error
	LookupTenantByFingerprint(ctx context.Context, fingerprint string) (uint, bool, error)
}

// TenantAuthMiddleware resolves the tenant for proxy requests from either:
//  1. X-TokenGate-Key — validates the key and retrieves tenant_id directly.
//     When X-Api-Key is also present, upserts the fingerprint→tenant_id mapping
//     so future requests without X-TokenGate-Key can still be attributed.
//  2. X-Api-Key fingerprint — looks up a previously registered fingerprint→tenant_id
//     mapping in Redis.
//
// Returns 401 if neither header resolves to a known tenant.
// Raw X-Api-Key values are never logged; only the first-8-char SHA256 prefix and
// key length are emitted for debugging.
func TenantAuthMiddleware(apiKeySvc *services.APIKeyService, fpSvc *services.FingerprintService) gin.HandlerFunc {
	return TenantAuthMiddlewareForTest(apiKeySvc, fpSvc)
}

// TenantAuthMiddlewareForTest is the testable variant that accepts the interface types.
// Production code should use TenantAuthMiddleware.
func TenantAuthMiddlewareForTest(apiKeySvc APIKeyValidatorForTest, fpSvc FingerprintStoreForTest) gin.HandlerFunc {
	return func(c *gin.Context) {
		tgKey := strings.TrimSpace(c.GetHeader("X-TokenGate-Key"))
		rawAPIKey := strings.TrimSpace(c.GetHeader("X-Api-Key"))

		// Pre-compute fingerprint whenever X-Api-Key is present.
		var fp, fpPrefix string
		var keyLen int
		if rawAPIKey != "" {
			keyLen = len(rawAPIKey)
			fp = services.ComputeAPIKeyFingerprint(rawAPIKey)
			fpPrefix = services.FingerprintDebugPrefix(fp)
			c.Set(ContextKeyFingerprint, fp)
		}

		// ── Path 1: X-TokenGate-Key present → authenticate directly ────────
		if tgKey != "" {
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
			c.Set("key_id", ak.KeyID)
			go apiKeySvc.TouchLastSeen(context.Background(), ak.KeyID)

			// If X-Api-Key is also present, register the fingerprint→tenant mapping
			// so subsequent requests without X-TokenGate-Key can be attributed.
			if fp != "" {
				if err := fpSvc.UpsertFingerprint(c.Request.Context(), fp, ak.TenantID); err != nil {
					log.Printf("fingerprint: upsert failed (fp_prefix=%s tenant=%d): %v", fpPrefix, ak.TenantID, err)
				} else {
					log.Printf("fingerprint: registered (fp_prefix=%s key_len=%d tenant=%d)", fpPrefix, keyLen, ak.TenantID)
				}
			}

			c.Next()
			return
		}

		// ── Path 2: X-Api-Key only → fingerprint lookup ─────────────────────
		if fp != "" {
			tenantID, found, err := fpSvc.LookupTenantByFingerprint(c.Request.Context(), fp)
			if err != nil {
				log.Printf("fingerprint: lookup error (fp_prefix=%s key_len=%d): %v", fpPrefix, keyLen, err)
			} else if found {
				log.Printf("fingerprint: attributed (fp_prefix=%s key_len=%d tenant=%d)", fpPrefix, keyLen, tenantID)
				c.Set("tenant_id", tenantID)
				c.Next()
				return
			} else {
				log.Printf("fingerprint: unattributed (fp_prefix=%s key_len=%d present=true)", fpPrefix, keyLen)
			}
		}

		// ── Neither header resolved a tenant ─────────────────────────────────
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{
			"type":    "tg_auth_missing",
			"message": "Authentication required. Provide X-TokenGate-Key, or send X-Api-Key after an initial request that includes X-TokenGate-Key.",
		}})
		c.Abort()
	}
}
