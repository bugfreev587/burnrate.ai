package middleware

import (
	"log"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/xiaoboyu/tokengate/api-server/internal/services"
)

// ContextKeyFingerprint is the Gin context key under which the computed
// API key fingerprint ("ak:<sha256-hex>") is stored for downstream handlers.
const ContextKeyFingerprint = "api_key_fingerprint"

// APIKeyFingerprintMiddleware derives a stable session fingerprint from the
// X-Api-Key header and manages its mapping to a tenant_id in Redis.
//
// When both X-Api-Key and X-TokenGate-Key are present:
//   - Validates X-TokenGate-Key to authenticate the tenant.
//   - Upserts fingerprint → tenant_id into Redis (best-effort).
//
// When only X-Api-Key is present (no X-TokenGate-Key):
//   - Looks up the fingerprint in Redis for attribution logging.
//   - Logs whether the request is attributed or unattributed.
//
// Security guarantees:
//   - Raw X-Api-Key values are never emitted to logs.
//   - Logs only contain: first-8-char SHA256 prefix, header length, found/missing status.
//
// This middleware never aborts the request — fingerprinting is always best-effort.
func APIKeyFingerprintMiddleware(fpSvc *services.FingerprintService, apiKeySvc *services.APIKeyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		rawKey := strings.TrimSpace(c.GetHeader("X-Api-Key"))
		if rawKey == "" {
			// No anchor header present — nothing to fingerprint.
			c.Next()
			return
		}

		keyLen := len(rawKey)
		fp := services.ComputeAPIKeyFingerprint(rawKey)
		prefix := services.FingerprintDebugPrefix(fp)

		// Store fingerprint in context so downstream handlers (e.g. proxy) can
		// include it in usage event messages for audit attribution.
		c.Set(ContextKeyFingerprint, fp)

		tgKey := strings.TrimSpace(c.GetHeader("X-TokenGate-Key"))
		if tgKey != "" {
			// Both headers present: validate the TokenGate key and register the
			// fingerprint → tenant binding so future requests can be attributed.
			ak, err := apiKeySvc.ValidateKey(c.Request.Context(), tgKey)
			if err != nil {
				// Don't block the request; key validation failure only means we
				// cannot register this fingerprint mapping right now.
				log.Printf("fingerprint: X-TokenGate-Key validation failed (fp_prefix=%s key_len=%d): skipping upsert", prefix, keyLen)
			} else {
				if upsertErr := fpSvc.UpsertFingerprint(c.Request.Context(), fp, ak.TenantID); upsertErr != nil {
					log.Printf("fingerprint: upsert failed (fp_prefix=%s tenant=%d): %v", prefix, ak.TenantID, upsertErr)
				} else {
					log.Printf("fingerprint: registered (fp_prefix=%s key_len=%d tenant=%d)", prefix, keyLen, ak.TenantID)
				}
			}
		} else {
			// Only X-Api-Key present: attempt fingerprint lookup for attribution.
			tenantID, found, err := fpSvc.LookupTenantByFingerprint(c.Request.Context(), fp)
			if err != nil {
				log.Printf("fingerprint: lookup error (fp_prefix=%s key_len=%d): %v", prefix, keyLen, err)
			} else if found {
				log.Printf("fingerprint: attributed (fp_prefix=%s key_len=%d tenant=%d)", prefix, keyLen, tenantID)
			} else {
				log.Printf("fingerprint: unattributed (fp_prefix=%s key_len=%d present=true)", prefix, keyLen)
			}
		}

		c.Next()
	}
}
