package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
)

const fpRedisKeyPrefix = "fp:"

// FingerprintService manages stable session fingerprints derived from X-Api-Key headers.
// Raw API key values are never stored or logged — only their SHA256 hashes are persisted.
type FingerprintService struct {
	cache *redis.Client
	ttl   time.Duration
}

// NewFingerprintService creates a FingerprintService backed by Redis.
// ttl controls how long a fingerprint→tenant_id mapping lives in Redis.
func NewFingerprintService(cache *redis.Client, ttl time.Duration) *FingerprintService {
	return &FingerprintService{cache: cache, ttl: ttl}
}

// ComputeAPIKeyFingerprint derives a stable fingerprint from a raw API key value.
// It trims whitespace from the key, SHA256-hashes the result, and returns "ak:<hex>".
// The raw key value is never retained or emitted.
func ComputeAPIKeyFingerprint(rawKey string) string {
	normalized := strings.TrimSpace(rawKey)
	sum := sha256.Sum256([]byte(normalized))
	return "ak:" + hex.EncodeToString(sum[:])
}

// FingerprintDebugPrefix returns the first 8 hex characters of the SHA256 portion
// of a fingerprint (i.e. after the "ak:" prefix) — safe for use in debug logs.
func FingerprintDebugPrefix(fp string) string {
	const pfxLen = len("ak:")
	if len(fp) <= pfxLen {
		return ""
	}
	hash := fp[pfxLen:]
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}

// UpsertFingerprint stores the fingerprint→tenantID mapping in Redis with the
// configured TTL. It is idempotent. A nil Redis client is a no-op.
func (s *FingerprintService) UpsertFingerprint(ctx context.Context, fingerprint string, tenantID uint) error {
	if s.cache == nil {
		return nil
	}
	return s.cache.Set(
		ctx,
		fpRedisKeyPrefix+fingerprint,
		strconv.FormatUint(uint64(tenantID), 10),
		s.ttl,
	).Err()
}

// LookupTenantByFingerprint resolves a tenant_id from a previously stored fingerprint.
// Returns (0, false, nil) when the fingerprint has no mapping (not found / expired).
func (s *FingerprintService) LookupTenantByFingerprint(ctx context.Context, fingerprint string) (uint, bool, error) {
	if s.cache == nil {
		return 0, false, nil
	}
	val, err := s.cache.Get(ctx, fpRedisKeyPrefix+fingerprint).Result()
	if err == redis.Nil {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("fingerprint lookup: %w", err)
	}
	id, err := strconv.ParseUint(val, 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("fingerprint parse tenant_id %q: %w", val, err)
	}
	return uint(id), true, nil
}
