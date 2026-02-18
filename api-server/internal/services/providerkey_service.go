package services

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/xiaoboyu/burnrate-ai/api-server/internal/models"
)

const providerKeyCacheTTL = 5 * time.Minute

var ErrProviderKeyNotFound = errors.New("provider key not found")
var ErrNoActiveKey = errors.New("no active provider key configured")

type cachedProviderKey struct {
	plaintext []byte
	expiresAt time.Time
}

type ProviderKeyService struct {
	db        *gorm.DB
	masterKey []byte // 32 bytes decoded from hex env var
	mu        sync.RWMutex
	cache     map[uint]cachedProviderKey // key_id → cachedProviderKey
}

// NewProviderKeyService decodes PROVIDER_KEY_ENCRYPTION_KEY (32-byte hex) and creates the service.
func NewProviderKeyService(db *gorm.DB, masterKeyHex string) (*ProviderKeyService, error) {
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
		cache:     make(map[uint]cachedProviderKey),
	}, nil
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
// Checks in-process cache first; caches result on miss.
func (s *ProviderKeyService) GetPlaintext(ctx context.Context, keyID uint) ([]byte, error) {
	// Check cache
	s.mu.RLock()
	if cached, ok := s.cache[keyID]; ok && time.Now().Before(cached.expiresAt) {
		s.mu.RUnlock()
		return cached.plaintext, nil
	}
	s.mu.RUnlock()

	// Load from DB
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

	// Decrypt DEK with master key
	dek, err := aesGCMDecrypt(s.masterKey, pk.EncryptedDEK, pk.DEKNonce)
	if err != nil {
		return nil, fmt.Errorf("decrypt DEK: %w", err)
	}

	// Decrypt key with DEK
	plaintext, err := aesGCMDecrypt(dek, pk.EncryptedKey, pk.KeyNonce)
	if err != nil {
		return nil, fmt.Errorf("decrypt key: %w", err)
	}

	// Store in cache
	s.mu.Lock()
	s.cache[keyID] = cachedProviderKey{
		plaintext: plaintext,
		expiresAt: time.Now().Add(providerKeyCacheTTL),
	}
	s.mu.Unlock()

	return plaintext, nil
}

// GetActiveKey looks up the active key for a tenant+provider pair and returns its plaintext.
func (s *ProviderKeyService) GetActiveKey(ctx context.Context, tenantID uint, provider string) ([]byte, error) {
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
	return s.GetPlaintext(ctx, settings.ActiveKeyID)
}

// Activate upserts TenantProviderSettings to point to the given key.
func (s *ProviderKeyService) Activate(ctx context.Context, tenantID uint, keyID uint) error {
	// Verify the key belongs to the tenant and is not revoked
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

	settings := models.TenantProviderSettings{
		TenantID:    tenantID,
		Provider:    pk.Provider,
		ActiveKeyID: keyID,
		UpdatedAt:   time.Now(),
	}
	result := s.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "provider"}},
			DoUpdates: clause.AssignmentColumns([]string{"active_key_id", "updated_at"}),
		}).
		Create(&settings)
	return result.Error
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
