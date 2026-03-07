package services

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/xiaoboyu/tokengate/api-server/internal/models"
	"github.com/xiaoboyu/tokengate/api-server/internal/provider"
)

var (
	ErrModelGroupNotFound = errors.New("model group not found")
	ErrModelGroupExists   = errors.New("model group with this name already exists")
)

// ModelGroupService handles CRUD for model group configurations and
// keeps the in-memory Router in sync with the database.
type ModelGroupService struct {
	db             *gorm.DB
	providerKeySvc *ProviderKeyService
	router         *provider.Router
}

// NewModelGroupService creates a new ModelGroupService.
func NewModelGroupService(db *gorm.DB, providerKeySvc *ProviderKeyService, router *provider.Router) *ModelGroupService {
	return &ModelGroupService{
		db:             db,
		providerKeySvc: providerKeySvc,
		router:         router,
	}
}

// LoadAllForTenant loads all enabled model groups for a tenant from the DB
// and registers them in the Router. Call this on server startup.
func (s *ModelGroupService) LoadAllForTenant(ctx context.Context, tenantID uint) error {
	var configs []models.ModelGroupConfig
	if err := s.db.WithContext(ctx).
		Preload("Deployments").
		Where("tenant_id = ? AND enabled = true", tenantID).
		Find(&configs).Error; err != nil {
		return fmt.Errorf("load model groups: %w", err)
	}

	for _, cfg := range configs {
		group, err := s.configToModelGroup(ctx, tenantID, &cfg)
		if err != nil {
			// Log but don't fail entire load
			continue
		}
		s.router.AddModelGroup(group)
	}
	return nil
}

// Create creates a new model group configuration and registers it in the Router.
func (s *ModelGroupService) Create(ctx context.Context, tenantID uint, cfg *models.ModelGroupConfig) error {
	cfg.TenantID = tenantID

	if err := s.db.WithContext(ctx).Create(cfg).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) || isDuplicateKeyError(err) {
			return ErrModelGroupExists
		}
		return fmt.Errorf("create model group: %w", err)
	}

	// Reload with deployments to get auto-generated IDs
	if err := s.db.WithContext(ctx).Preload("Deployments").First(cfg, cfg.ID).Error; err != nil {
		return fmt.Errorf("reload model group: %w", err)
	}

	if cfg.Enabled {
		group, err := s.configToModelGroup(ctx, tenantID, cfg)
		if err != nil {
			return nil // created in DB but router sync failed; will be retried
		}
		s.router.AddModelGroup(group)
	}
	return nil
}

// Update updates an existing model group configuration and refreshes the Router.
func (s *ModelGroupService) Update(ctx context.Context, tenantID uint, groupID uint, updates *models.ModelGroupConfig) error {
	var existing models.ModelGroupConfig
	if err := s.db.WithContext(ctx).
		Preload("Deployments").
		Where("id = ? AND tenant_id = ?", groupID, tenantID).
		First(&existing).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrModelGroupNotFound
		}
		return fmt.Errorf("load model group: %w", err)
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Update the main config
		if err := tx.Model(&existing).Updates(map[string]interface{}{
			"name":        updates.Name,
			"strategy":    updates.Strategy,
			"description": updates.Description,
			"enabled":     updates.Enabled,
		}).Error; err != nil {
			return err
		}

		// Replace deployments: delete old, insert new
		if err := tx.Where("model_group_id = ?", groupID).Delete(&models.ModelGroupDeployment{}).Error; err != nil {
			return err
		}
		for i := range updates.Deployments {
			updates.Deployments[i].ModelGroupID = groupID
			updates.Deployments[i].ID = 0 // reset for insert
		}
		if len(updates.Deployments) > 0 {
			if err := tx.Create(&updates.Deployments).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("update model group: %w", err)
	}

	// Refresh router
	var refreshed models.ModelGroupConfig
	if err := s.db.WithContext(ctx).Preload("Deployments").First(&refreshed, groupID).Error; err != nil {
		return nil
	}
	if refreshed.Enabled {
		group, err := s.configToModelGroup(ctx, tenantID, &refreshed)
		if err == nil {
			s.router.AddModelGroup(group)
		}
	} else {
		s.router.RemoveModelGroup(existing.Name)
	}
	return nil
}

// Delete deletes a model group and removes it from the Router.
func (s *ModelGroupService) Delete(ctx context.Context, tenantID uint, groupID uint) error {
	var cfg models.ModelGroupConfig
	if err := s.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", groupID, tenantID).First(&cfg).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrModelGroupNotFound
		}
		return fmt.Errorf("load model group: %w", err)
	}

	if err := s.db.WithContext(ctx).Delete(&cfg).Error; err != nil {
		return fmt.Errorf("delete model group: %w", err)
	}

	s.router.RemoveModelGroup(cfg.Name)
	return nil
}

// List returns all model groups for a tenant.
func (s *ModelGroupService) List(ctx context.Context, tenantID uint) ([]models.ModelGroupConfig, error) {
	var configs []models.ModelGroupConfig
	if err := s.db.WithContext(ctx).
		Preload("Deployments").
		Where("tenant_id = ?", tenantID).
		Order("created_at DESC").
		Find(&configs).Error; err != nil {
		return nil, fmt.Errorf("list model groups: %w", err)
	}
	return configs, nil
}

// GetByID returns a single model group by ID.
func (s *ModelGroupService) GetByID(ctx context.Context, tenantID uint, groupID uint) (*models.ModelGroupConfig, error) {
	var cfg models.ModelGroupConfig
	if err := s.db.WithContext(ctx).
		Preload("Deployments").
		Where("id = ? AND tenant_id = ?", groupID, tenantID).
		First(&cfg).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrModelGroupNotFound
		}
		return nil, fmt.Errorf("get model group: %w", err)
	}
	return &cfg, nil
}

// configToModelGroup converts a DB config into a provider.ModelGroup with resolved API keys.
func (s *ModelGroupService) configToModelGroup(ctx context.Context, tenantID uint, cfg *models.ModelGroupConfig) (*provider.ModelGroup, error) {
	group := &provider.ModelGroup{
		Name:     cfg.Name,
		Strategy: cfg.Strategy,
	}

	for _, dep := range cfg.Deployments {
		if !dep.Enabled {
			continue
		}

		apiKey, err := s.resolveDeploymentKey(ctx, tenantID, dep.Provider, dep.ProviderKeyID)
		if err != nil {
			return nil, fmt.Errorf("resolve key for deployment %s/%s: %w", dep.Provider, dep.Model, err)
		}

		group.Deployments = append(group.Deployments, provider.Deployment{
			ID:              fmt.Sprintf("mg-%d-dep-%d", cfg.ID, dep.ID),
			Provider:        dep.Provider,
			Model:           dep.Model,
			APIKey:          string(apiKey),
			Priority:        dep.Priority,
			Weight:          dep.Weight,
			CostPer1KInput:  dep.CostPer1KInput,
			CostPer1KOutput: dep.CostPer1KOutput,
		})
	}

	if len(group.Deployments) == 0 {
		return nil, fmt.Errorf("model group %q has no enabled deployments with valid keys", cfg.Name)
	}
	return group, nil
}

// resolveDeploymentKey returns the plaintext API key for a deployment.
// If providerKeyID is set, use that specific key; otherwise use the tenant's active key.
func (s *ModelGroupService) resolveDeploymentKey(ctx context.Context, tenantID uint, providerName string, providerKeyID *uint) ([]byte, error) {
	if providerKeyID != nil && *providerKeyID > 0 {
		return s.providerKeySvc.GetPlaintext(ctx, *providerKeyID)
	}
	return s.providerKeySvc.GetActiveKey(ctx, tenantID, providerName)
}

// isDuplicateKeyError checks for PostgreSQL unique constraint violation.
func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return len(msg) > 0 && (contains(msg, "duplicate key") || contains(msg, "UNIQUE constraint"))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
