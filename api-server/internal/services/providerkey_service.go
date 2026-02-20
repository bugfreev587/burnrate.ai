package services

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

// In-process plaintext cache: sliding idle window + absolute hard ceiling.
const (
	providerKeyIdleTTL = 30 * time.Second // TTL resets on each cache hit
	providerKeyHardTTL = 5 * time.Minute  // absolute ceiling regardless of hit rate
)

// Redis TPS cache: same sliding + hard pattern, hard_expiry stored in the value.
const (
	tpsIdleTTL = 30 * time.Second // Redis TTL reset on each cache hit via EXPIRE
	tpsHardTTL = 5 * time.Minute  // stored in JSON; forces DB re-fetch when elapsed
)

var ErrProviderKeyNotFound = errors.New("provider key not found")
var ErrNoActiveKey = errors.New("no active provider key configured")

// tpsCacheEntry is the value stored under tpsCacheKey in Redis.
// HardExpiry enforces an absolute max lifetime even for hot keys.
type tpsCacheEntry struct {
	ActiveKeyID   uint      `json:"active_key_id"`
	PolicyVersion int       `json:"policy_version"`
	HardExpiry    time.Time `json:"hard_expiry"`
}

// cachedProviderKey holds a decrypted plaintext with a sliding idle window
// and an absolute hard ceiling.
type cachedProviderKey struct {
	plaintext  []byte
	idleExpiry time.Time // extended on each hit
	hardExpiry time.Time // set once at write time, never extended
}

type ProviderKeyService struct {
	db        *gorm.DB
	masterKey []byte // 32 bytes decoded from hex env var
	rdb       *redis.Client
	mu        sync.RWMutex
	cache     map[uint]cachedProviderKey // key_id → cachedProviderKey
}

// NewProviderKeyService decodes PROVIDER_KEY_ENCRYPTION_KEY (32-byte hex) and creates the service.
// rdb may be nil; caching is skipped when Redis is unavailable.
func NewProviderKeyService(db *gorm.DB, masterKeyHex string, rdb *redis.Client) (*ProviderKeyService, error) {
	if masterKeyHex == "" {
		return nil, fmt.Errorf("PROVIDER_KEY_ENCRYPTION_KEY is not set; refusing to start without encryption key")
	}
	key, err := hex.DecodeString(masterKeyHex)
	if err != nil {
		return nil, fmt.Errorf("PROVIDER_KEY_ENCRYPTION_KEY is not valid hex: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("PROVIDER_KEY_ENCRYPTION_KEY must be 32 bytes (64 hex chars), got %d bytes", len(key))
	}
	return &ProviderKeyService{
		db:        db,
		masterKey: key,
		rdb:       rdb,
		cache:     make(map[uint]cachedProviderKey),
	}, nil
}

// tpsCacheKey returns the Redis key for a tenant+provider TPS cache entry.
func tpsCacheKey(tenantID uint, provider string) string {
	return fmt.Sprintf("tps:%d:%s", tenantID, provider)
}

// cacheTPS writes a TenantProviderSettings entry to Redis with the idle TTL.
// HardExpiry is embedded in the value so reads can enforce the absolute ceiling.
func (s *ProviderKeyService) cacheTPS(ctx context.Context, settings *models.TenantProviderSettings) {
	if s.rdb == nil {
		return
	}
	entry := tpsCacheEntry{
		ActiveKeyID:   settings.ActiveKeyID,
		PolicyVersion: settings.PolicyVersion,
		HardExpiry:    time.Now().Add(tpsHardTTL),
	}
	if b, err := json.Marshal(entry); err == nil {
		s.rdb.Set(ctx, tpsCacheKey(settings.TenantID, settings.Provider), b, tpsIdleTTL)
	}
}

// invalidateTPS removes the Redis TPS cache entry for a tenant+provider pair,
// forcing the next GetActiveKey call to re-fetch from DB.
func (s *ProviderKeyService) invalidateTPS(ctx context.Context, tenantID uint, provider string) {
	if s.rdb != nil {
		s.rdb.Del(ctx, tpsCacheKey(tenantID, provider))
	}
}

// Store encrypts the plaintext key with envelope encryption and persists it.
func (s *ProviderKeyService) Store(ctx context.Context, tenantID uint, provider, label, plaintextKey string) (*models.ProviderKey, error) {
	// Generate a random 32-byte DEK
	dek := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return nil, fmt.Errorf("generate DEK: %w", err)
	}

	// Encrypt the plaintext key with the DEK
	encKey, keyNonce, err := aesGCMEncrypt(dek, []byte(plaintextKey))
	if err != nil {
		return nil, fmt.Errorf("encrypt key with DEK: %w", err)
	}

	// Encrypt the DEK with the master key
	encDEK, dekNonce, err := aesGCMEncrypt(s.masterKey, dek)
	if err != nil {
		return nil, fmt.Errorf("encrypt DEK with master key: %w", err)
	}

	pk := &models.ProviderKey{
		TenantID:     tenantID,
		Provider:     provider,
		Label:        label,
		EncryptedKey: encKey,
		KeyNonce:     keyNonce,
		EncryptedDEK: encDEK,
		DEKNonce:     dekNonce,
		Revoked:      false,
	}
	if err := s.db.WithContext(ctx).Create(pk).Error; err != nil {
		return nil, fmt.Errorf("store provider key: %w", err)
	}
	return pk, nil
}

// GetPlaintext decrypts and returns a provider key's plaintext value.
// Uses a sliding idle TTL (reset on each hit) bounded by a hard ceiling
// (set once at write time, never extended). Both must be satisfied for a
// cache hit to be valid.
func (s *ProviderKeyService) GetPlaintext(ctx context.Context, keyID uint) ([]byte, error) {
	now := time.Now()

	// Fast path: in-process cache.
	s.mu.RLock()
	cached, ok := s.cache[keyID]
	s.mu.RUnlock()

	if ok {
		if now.Before(cached.idleExpiry) && now.Before(cached.hardExpiry) {
			// Valid hit — extend the idle window (hard ceiling is unchanged).
			s.mu.Lock()
			if c, still := s.cache[keyID]; still {
				c.idleExpiry = time.Now().Add(providerKeyIdleTTL)
				s.cache[keyID] = c
			}
			s.mu.Unlock()
			return cached.plaintext, nil
		}
		// Either idle or hard expired — evict stale entry.
		s.mu.Lock()
		delete(s.cache, keyID)
		s.mu.Unlock()
	}

	// Slow path: DB decrypt.
	var pk models.ProviderKey
	if err := s.db.WithContext(ctx).First(&pk, keyID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrProviderKeyNotFound
		}
		return nil, fmt.Errorf("load provider key: %w", err)
	}
	if pk.Revoked {
		return nil, ErrProviderKeyNotFound
	}

	dek, err := aesGCMDecrypt(s.masterKey, pk.EncryptedDEK, pk.DEKNonce)
	if err != nil {
		return nil, fmt.Errorf("decrypt DEK: %w", err)
	}
	plaintext, err := aesGCMDecrypt(dek, pk.EncryptedKey, pk.KeyNonce)
	if err != nil {
		return nil, fmt.Errorf("decrypt key: %w", err)
	}

	now = time.Now()
	s.mu.Lock()
	s.cache[keyID] = cachedProviderKey{
		plaintext:  plaintext,
		idleExpiry: now.Add(providerKeyIdleTTL),
		hardExpiry: now.Add(providerKeyHardTTL),
	}
	s.mu.Unlock()

	return plaintext, nil
}

// GetActiveKey looks up the active key for a tenant+provider pair and returns its plaintext.
// It checks the Redis TPS cache first to avoid a DB round trip on every proxy request.
// The TPS cache is invalidated on every Activate or Rotate call.
func (s *ProviderKeyService) GetActiveKey(ctx context.Context, tenantID uint, provider string) ([]byte, error) {
	// Fast path: Redis TPS cache.
	if s.rdb != nil {
		key := tpsCacheKey(tenantID, provider)
		if raw, err := s.rdb.Get(ctx, key).Result(); err == nil {
			var entry tpsCacheEntry
			if json.Unmarshal([]byte(raw), &entry) == nil && entry.ActiveKeyID != 0 {
				if time.Now().Before(entry.HardExpiry) {
					// Valid hit — slide the idle TTL.
					s.rdb.Expire(ctx, key, tpsIdleTTL)
					return s.GetPlaintext(ctx, entry.ActiveKeyID)
				}
				// Hard expired — drop the stale entry and fall through to DB.
				s.rdb.Del(ctx, key)
			}
		}
	}

	// Slow path: DB.
	var settings models.TenantProviderSettings
	err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND provider = ?", tenantID, provider).
		First(&settings).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNoActiveKey
		}
		return nil, fmt.Errorf("load tenant provider settings: %w", err)
	}
	if settings.ActiveKeyID == 0 {
		return nil, ErrNoActiveKey
	}

	// Populate cache for next request.
	s.cacheTPS(ctx, &settings)

	return s.GetPlaintext(ctx, settings.ActiveKeyID)
}

// Activate upserts TenantProviderSettings to point to the given key and bumps policy_version.
func (s *ProviderKeyService) Activate(ctx context.Context, tenantID uint, keyID uint) error {
	// Verify the key belongs to the tenant and is not revoked.
	var pk models.ProviderKey
	if err := s.db.WithContext(ctx).First(&pk, keyID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrProviderKeyNotFound
		}
		return fmt.Errorf("load provider key: %w", err)
	}
	if pk.TenantID != tenantID {
		return ErrProviderKeyNotFound
	}
	if pk.Revoked {
		return fmt.Errorf("cannot activate a revoked key")
	}

	// Upsert: INSERT with policy_version=1; on conflict bump policy_version atomically.
	result := s.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "tenant_id"}, {Name: "provider"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"active_key_id":  keyID,
				"updated_at":     time.Now(),
				"policy_version": gorm.Expr("tenant_provider_settings.policy_version + 1"),
			}),
		}).
		Create(&models.TenantProviderSettings{
			TenantID:      tenantID,
			Provider:      pk.Provider,
			ActiveKeyID:   keyID,
			PolicyVersion: 1,
			UpdatedAt:     time.Now(),
		})
	if result.Error != nil {
		return result.Error
	}

	// Invalidate TPS cache so the next request re-fetches the new policy_version from DB.
	s.invalidateTPS(ctx, tenantID, pk.Provider)
	return nil
}

// Rotate atomically stores a new provider key, activates it, and revokes the old key
// — all inside a single DB transaction. The old key's plaintext is evicted from cache
// after the transaction commits.
func (s *ProviderKeyService) Rotate(ctx context.Context, tenantID uint, oldKeyID uint, label, plaintextKey string) (*models.ProviderKey, error) {
	// Verify old key belongs to this tenant and is not already revoked.
	var oldKey models.ProviderKey
	if err := s.db.WithContext(ctx).First(&oldKey, oldKeyID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrProviderKeyNotFound
		}
		return nil, fmt.Errorf("load old provider key: %w", err)
	}
	if oldKey.TenantID != tenantID {
		return nil, ErrProviderKeyNotFound
	}
	if oldKey.Revoked {
		return nil, fmt.Errorf("cannot rotate an already-revoked key")
	}

	// Encrypt the new key outside the transaction (no I/O inside tx).
	dek := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return nil, fmt.Errorf("generate DEK: %w", err)
	}
	encKey, keyNonce, err := aesGCMEncrypt(dek, []byte(plaintextKey))
	if err != nil {
		return nil, fmt.Errorf("encrypt key with DEK: %w", err)
	}
	encDEK, dekNonce, err := aesGCMEncrypt(s.masterKey, dek)
	if err != nil {
		return nil, fmt.Errorf("encrypt DEK with master key: %w", err)
	}

	var newKey models.ProviderKey

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. Store the new key.
		newKey = models.ProviderKey{
			TenantID:     tenantID,
			Provider:     oldKey.Provider,
			Label:        label,
			EncryptedKey: encKey,
			KeyNonce:     keyNonce,
			EncryptedDEK: encDEK,
			DEKNonce:     dekNonce,
			Revoked:      false,
		}
		if err := tx.Create(&newKey).Error; err != nil {
			return fmt.Errorf("store new provider key: %w", err)
		}

		// 2. Atomically point TenantProviderSettings at the new key and bump policy_version.
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "tenant_id"}, {Name: "provider"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"active_key_id":  newKey.ID,
				"updated_at":     time.Now(),
				"policy_version": gorm.Expr("tenant_provider_settings.policy_version + 1"),
			}),
		}).Create(&models.TenantProviderSettings{
			TenantID:      tenantID,
			Provider:      oldKey.Provider,
			ActiveKeyID:   newKey.ID,
			PolicyVersion: 1,
			UpdatedAt:     time.Now(),
		}).Error; err != nil {
			return fmt.Errorf("activate new provider key: %w", err)
		}

		// 3. Revoke the old key.
		if err := tx.Model(&oldKey).Update("revoked", true).Error; err != nil {
			return fmt.Errorf("revoke old provider key: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Evict old key from in-process cache after the transaction commits.
	s.mu.Lock()
	delete(s.cache, oldKeyID)
	s.mu.Unlock()

	// Invalidate Redis TPS cache so every pod picks up the new active_key_id
	// and policy_version on the very next request (no wait for TTL expiry).
	s.invalidateTPS(ctx, tenantID, oldKey.Provider)

	return &newKey, nil
}

// Revoke marks a provider key as revoked, evicts it from cache, and clears TenantProviderSettings
// if it was the active key.
func (s *ProviderKeyService) Revoke(ctx context.Context, tenantID uint, keyID uint) error {
	var pk models.ProviderKey
	if err := s.db.WithContext(ctx).First(&pk, keyID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrProviderKeyNotFound
		}
		return fmt.Errorf("load provider key: %w", err)
	}
	if pk.TenantID != tenantID {
		return ErrProviderKeyNotFound
	}

	if err := s.db.WithContext(ctx).Model(&pk).Update("revoked", true).Error; err != nil {
		return fmt.Errorf("revoke provider key: %w", err)
	}

	// Evict from cache
	s.mu.Lock()
	delete(s.cache, keyID)
	s.mu.Unlock()

	// Clear TenantProviderSettings if this was the active key
	s.db.WithContext(ctx).
		Where("tenant_id = ? AND provider = ? AND active_key_id = ?", tenantID, pk.Provider, keyID).
		Delete(&models.TenantProviderSettings{})

	return nil
}

// List returns all non-revoked provider keys for a tenant (no plaintext).
func (s *ProviderKeyService) List(ctx context.Context, tenantID uint) ([]models.ProviderKey, error) {
	var keys []models.ProviderKey
	if err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND revoked = false", tenantID).
		Order("created_at DESC").
		Find(&keys).Error; err != nil {
		return nil, fmt.Errorf("list provider keys: %w", err)
	}
	return keys, nil
}

// GetActiveSettings returns the TenantProviderSettings for a given tenant+provider.
func (s *ProviderKeyService) GetActiveSettings(ctx context.Context, tenantID uint, provider string) (*models.TenantProviderSettings, error) {
	var settings models.TenantProviderSettings
	err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND provider = ?", tenantID, provider).
		First(&settings).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &settings, nil
}

// aesGCMEncrypt encrypts plaintext with key using AES-256-GCM.
// Returns (ciphertext, nonce, error).
func aesGCMEncrypt(key, plaintext []byte) ([]byte, []byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

// aesGCMDecrypt decrypts ciphertext with key and nonce using AES-256-GCM.
func aesGCMDecrypt(key, ciphertext, nonce []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, ciphertext, nil)
}
