package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"

	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

// TenantLookup is implemented by TenantService; used by middleware to allow mock injection in tests.
type TenantLookup interface {
	GetTenantBySlug(ctx context.Context, slug string) (*models.Tenant, error)
}

var (
	ErrTenantNotFound  = errors.New("tenant not found")
	ErrTenantSuspended = errors.New("tenant suspended")
)

const tenantSlugCacheTTL = 10 * time.Minute

type tenantCacheEntry struct {
	ID     uint   `json:"id"`
	Name   string `json:"name"`
	Slug   string `json:"slug"`
	Status string `json:"status"`
}

type TenantService struct {
	db    *gorm.DB
	cache *redis.Client
}

func NewTenantService(db *gorm.DB, cache *redis.Client) *TenantService {
	return &TenantService{db: db, cache: cache}
}

// GetTenantBySlug looks up a tenant by slug using Redis as a read-through cache.
// Returns ErrTenantNotFound if no matching tenant exists.
// Returns ErrTenantSuspended if the tenant's status is not "active".
func (s *TenantService) GetTenantBySlug(ctx context.Context, slug string) (*models.Tenant, error) {
	cacheKey := "tenant_by_slug:" + slug

	if s.cache != nil {
		if raw, err := s.cache.Get(ctx, cacheKey).Result(); err == nil {
			var entry tenantCacheEntry
			if json.Unmarshal([]byte(raw), &entry) == nil {
				if entry.Status != models.StatusActive {
					return nil, ErrTenantSuspended
				}
				return &models.Tenant{ID: entry.ID, Name: entry.Name, Slug: entry.Slug, Status: entry.Status}, nil
			}
		}
	}

	var tenant models.Tenant
	if err := s.db.WithContext(ctx).Where("slug = ?", slug).First(&tenant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTenantNotFound
		}
		return nil, fmt.Errorf("db lookup: %w", err)
	}

	// Cache suspended tenants too — avoids DB hammering on probes.
	// Not-found results are NOT cached (avoids negative-cache complexity).
	if s.cache != nil {
		entry := tenantCacheEntry{ID: tenant.ID, Name: tenant.Name, Slug: tenant.Slug, Status: tenant.Status}
		if b, err := json.Marshal(entry); err == nil {
			s.cache.Set(ctx, cacheKey, b, tenantSlugCacheTTL)
		}
	}

	if tenant.Status != models.StatusActive {
		return nil, ErrTenantSuspended
	}
	return &tenant, nil
}
