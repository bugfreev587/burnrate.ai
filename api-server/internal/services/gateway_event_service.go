package services

import (
	"context"
	"log/slog"

	"gorm.io/gorm"

	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

// GatewayEventService records blocked gateway events (rate limit, budget exceeded).
type GatewayEventService struct {
	db *gorm.DB
}

// NewGatewayEventService creates a new GatewayEventService.
func NewGatewayEventService(db *gorm.DB) *GatewayEventService {
	return &GatewayEventService{db: db}
}

// Record persists a gateway event in a fire-and-forget goroutine.
func (s *GatewayEventService) Record(ctx context.Context, event *models.GatewayEvent) {
	go func() {
		if err := s.db.WithContext(context.Background()).Create(event).Error; err != nil {
			slog.Error("gateway_event_record_failed",
				"tenant_id", event.TenantID,
				"event_type", event.EventType,
				"error", err,
			)
		}
	}()
}
