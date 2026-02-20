package services

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

type APIKeyService struct {
	db       *gorm.DB
	pepper   []byte
	cache    *redis.Client
	cacheTTL time.Duration
}

type apiKeyCacheData struct {
	KeyID      string     `json:"key_id"`
	TenantID   uint       `json:"tenant_id"`
	Label      string     `json:"label"`
	Revoked    bool       `json:"revoked"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	Scopes     []string   `json:"scopes"`
	Salt       string     `json:"salt"`        // base64
	SecretHash string     `json:"secret_hash"` // base64
}

func NewAPIKeyService(db *gorm.DB, pepper []byte, cache *redis.Client, ttl time.Duration) *APIKeyService {
	return &APIKeyService{db: db, pepper: pepper, cache: cache, cacheTTL: ttl}
}

// ErrAPIKeyLimitReached is returned when a tenant has reached their active key limit.
type ErrAPIKeyLimitReached struct {
	Limit   int
	Current int64
}

func (e *ErrAPIKeyLimitReached) Error() string {
	return fmt.Sprintf("api key limit reached: %d/%d active keys", e.Current, e.Limit)
}

// CreateKey creates a new tenant-scoped API key and returns (keyID, secret, error).
// The secret is shown only once and is never stored in plain text.
func (s *APIKeyService) CreateKey(ctx context.Context, tenantID uint, label string, scopes []string, expiresAt *time.Time) (string, string, error) {
	// Fetch tenant to read the limit
	var tenant models.Tenant
	if err := s.db.WithContext(ctx).First(&tenant, tenantID).Error; err != nil {
		return "", "", fmt.Errorf("tenant not found: %w", err)
	}

	// Count active keys: non-revoked and not yet expired
	var activeCount int64
	s.db.WithContext(ctx).Model(&models.APIKey{}).
		Where("tenant_id = ? AND revoked = false AND (expires_at IS NULL OR expires_at > NOW())", tenantID).
		Count(&activeCount)

	limit := tenant.MaxAPIKeys
	if limit <= 0 {
		limit = 5 // safety fallback
	}
	if activeCount >= int64(limit) {
		return "", "", &ErrAPIKeyLimitReached{Limit: limit, Current: activeCount}
	}
	rawSecret := make([]byte, 32)
	if _, err := rand.Read(rawSecret); err != nil {
		return "", "", err
	}
	secret := base64.RawURLEncoding.EncodeToString(rawSecret)

	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", "", err
	}

	mac := hmac.New(sha256.New, s.pepper)
	mac.Write(salt)
	mac.Write([]byte(secret))
	hash := mac.Sum(nil)

	kid := "tg_" + uuid.New().String()
	ak := models.APIKey{
		TenantID:   tenantID,
		KeyID:      kid,
		Label:      label,
		Salt:       salt,
		SecretHash: hash,
		Scopes:     scopes,
		Revoked:    false,
		ExpiresAt:  expiresAt,
	}
	if err := s.db.Create(&ak).Error; err != nil {
		return "", "", err
	}
	s.cacheKey(ctx, &ak)
	return kid, secret, nil
}

// ValidateKey validates a key in "keyID:secret" format and returns the APIKey record.
func (s *APIKeyService) ValidateKey(ctx context.Context, presented string) (*models.APIKey, error) {
	keyID, secret, err := splitKey(presented)
	if err != nil {
		return nil, err
	}

	var ak models.APIKey
	cacheKey := "apikey:" + keyID

	// Cache-first lookup
	if s.cache != nil {
		if raw, err := s.cache.Get(ctx, cacheKey).Result(); err == nil {
			var cd apiKeyCacheData
			if json.Unmarshal([]byte(raw), &cd) == nil {
				if cd.Revoked {
					return nil, fmt.Errorf("key revoked")
				}
				if cd.ExpiresAt != nil && cd.ExpiresAt.Before(time.Now()) {
					return nil, fmt.Errorf("key expired")
				}
				salt, _ := base64.StdEncoding.DecodeString(cd.Salt)
				storedHash, _ := base64.StdEncoding.DecodeString(cd.SecretHash)
				if !s.verifySecret(salt, storedHash, secret) {
					return nil, fmt.Errorf("invalid key: hash mismatch")
				}
				return &models.APIKey{
					TenantID:   cd.TenantID,
					KeyID:      cd.KeyID,
					Label:      cd.Label,
					Salt:       salt,
					SecretHash: storedHash,
					Scopes:     cd.Scopes,
					Revoked:    cd.Revoked,
					ExpiresAt:  cd.ExpiresAt,
				}, nil
			}
		}
	}

	// DB fallback
	if err := s.db.Where("key_id = ?", keyID).First(&ak).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("key not found")
		}
		return nil, fmt.Errorf("db error: %w", err)
	}
	if ak.Revoked {
		return nil, fmt.Errorf("key revoked")
	}
	if ak.ExpiresAt != nil && ak.ExpiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("key expired")
	}
	if !s.verifySecret(ak.Salt, ak.SecretHash, secret) {
		return nil, fmt.Errorf("invalid key: hash mismatch")
	}

	s.cacheKey(ctx, &ak)
	return &ak, nil
}

// RevokeKey marks a tenant's key as revoked and removes it from cache.
func (s *APIKeyService) RevokeKey(ctx context.Context, tenantID uint, keyID string) error {
	res := s.db.Model(&models.APIKey{}).
		Where("key_id = ? AND tenant_id = ?", keyID, tenantID).
		Update("revoked", true)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("key not found")
	}
	if s.cache != nil {
		s.cache.Del(ctx, "apikey:"+keyID)
	}
	// Remove any pricing config association for this key
	s.db.Where("key_id = ?", keyID).Delete(&models.APIKeyConfig{})
	return nil
}

// ListKeys returns all non-revoked API keys for a tenant.
func (s *APIKeyService) ListKeys(ctx context.Context, tenantID uint) ([]models.APIKey, error) {
	var keys []models.APIKey
	if err := s.db.Where("tenant_id = ? AND revoked = ?", tenantID, false).Find(&keys).Error; err != nil {
		return nil, err
	}
	return keys, nil
}

// TouchLastSeen records that the key was seen at most once per minute.
// It is safe to call concurrently and is a no-op when Redis is unavailable.
func (s *APIKeyService) TouchLastSeen(ctx context.Context, keyID string) {
	now := time.Now()
	if s.cache != nil {
		// SET NX EX 60: only succeeds if the key is not already set.
		set, err := s.cache.SetNX(ctx, "apikey:touch:"+keyID, "1", 60*time.Second).Result()
		if err != nil || !set {
			return // Redis unavailable or already touched within the last minute
		}
	}
	s.db.WithContext(ctx).Model(&models.APIKey{}).
		Where("key_id = ?", keyID).
		Update("last_seen_at", now)
}

func (s *APIKeyService) verifySecret(salt, storedHash []byte, secret string) bool {
	mac := hmac.New(sha256.New, s.pepper)
	mac.Write(salt)
	mac.Write([]byte(secret))
	expected := mac.Sum(nil)
	return hmac.Equal(expected, storedHash)
}

func (s *APIKeyService) cacheKey(ctx context.Context, ak *models.APIKey) {
	if s.cache == nil {
		return
	}
	cd := apiKeyCacheData{
		KeyID:      ak.KeyID,
		TenantID:   ak.TenantID,
		Label:      ak.Label,
		Revoked:    ak.Revoked,
		ExpiresAt:  ak.ExpiresAt,
		Scopes:     ak.Scopes,
		Salt:       base64.StdEncoding.EncodeToString(ak.Salt),
		SecretHash: base64.StdEncoding.EncodeToString(ak.SecretHash),
	}
	if b, err := json.Marshal(cd); err == nil {
		s.cache.Set(ctx, "apikey:"+ak.KeyID, b, s.cacheTTL)
	}
}

func splitKey(presented string) (keyID, secret string, err error) {
	for i, b := range []byte(presented) {
		if b == ':' {
			return presented[:i], presented[i+1:], nil
		}
	}
	return "", "", fmt.Errorf("bad key format: expected 'keyid:secret'")
}
