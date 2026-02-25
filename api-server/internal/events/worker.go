package events

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"

	"github.com/xiaoboyu/tokengate/api-server/internal/models"
	"github.com/xiaoboyu/tokengate/api-server/internal/pricing"
	"github.com/xiaoboyu/tokengate/api-server/internal/services"
)

const consumerGroup = "tokengate:usage:workers"
const consumerName = "worker-1"

// UsageWorker consumes from the Redis stream and processes usage events.
type UsageWorker struct {
	rdb           *redis.Client
	pricingEngine *pricing.PricingEngine
	usageSvc      *services.UsageLogService
}

// NewUsageWorker creates a new UsageWorker.
func NewUsageWorker(rdb *redis.Client, pricingEngine *pricing.PricingEngine, usageSvc *services.UsageLogService) *UsageWorker {
	return &UsageWorker{
		rdb:           rdb,
		pricingEngine: pricingEngine,
		usageSvc:      usageSvc,
	}
}

// Run starts the Redis Streams consumer loop. It blocks until ctx is cancelled.
func (w *UsageWorker) Run(ctx context.Context) {
	// Create consumer group; ignore BUSYGROUP error if it already exists.
	err := w.rdb.XGroupCreateMkStream(ctx, streamName, consumerGroup, "$").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		log.Printf("usageworker: XGROUP CREATE: %v", err)
	}

	log.Printf("usageworker: started, reading from stream=%s group=%s", streamName, consumerGroup)

	for {
		select {
		case <-ctx.Done():
			log.Println("usageworker: context cancelled, stopping")
			return
		default:
		}

		streams, err := w.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    consumerGroup,
			Consumer: consumerName,
			Streams:  []string{streamName, ">"},
			Count:    10,
			Block:    5 * time.Second,
		}).Result()
		if err != nil {
			if err == redis.Nil || err.Error() == "redis: nil" {
				continue // no messages, try again
			}
			if ctx.Err() != nil {
				return
			}
			log.Printf("usageworker: XREADGROUP error: %v", err)
			time.Sleep(time.Second)
			continue
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				if err := w.process(ctx, msg); err != nil {
					log.Printf("usageworker: process msg %s: %v", msg.ID, err)
					// Don't ACK on failure; let it be redelivered
					continue
				}
				// ACK on success
				if err := w.rdb.XAck(ctx, streamName, consumerGroup, msg.ID).Err(); err != nil {
					log.Printf("usageworker: XACK msg %s: %v", msg.ID, err)
				}
			}
		}
	}
}

// process handles a single stream message.
func (w *UsageWorker) process(ctx context.Context, msg redis.XMessage) error {
	v := msg.Values

	tenantID64, err := strconv.ParseUint(fmt.Sprintf("%v", v["tenant_id"]), 10, 64)
	if err != nil {
		return fmt.Errorf("parse tenant_id: %w", err)
	}
	tenantID := uint(tenantID64)

	inputTokens, _ := strconv.ParseInt(fmt.Sprintf("%v", v["input_tokens"]), 10, 64)
	outputTokens, _ := strconv.ParseInt(fmt.Sprintf("%v", v["output_tokens"]), 10, 64)
	cacheCreationTokens, _ := strconv.ParseInt(fmt.Sprintf("%v", v["cache_creation_tokens"]), 10, 64)
	cacheReadTokens, _ := strconv.ParseInt(fmt.Sprintf("%v", v["cache_read_tokens"]), 10, 64)

	keyID := fmt.Sprintf("%v", v["key_id"])
	provider := fmt.Sprintf("%v", v["provider"])
	model := fmt.Sprintf("%v", v["model"])
	messageID := fmt.Sprintf("%v", v["message_id"])
	apiKeyFingerprint := fmt.Sprintf("%v", v["api_key_fingerprint"])
	if apiKeyFingerprint == "<nil>" {
		apiKeyFingerprint = ""
	}
	apiUsageBilled := fmt.Sprintf("%v", v["api_usage_billed"]) == "true"

	var ts time.Time
	if tsStr, ok := v["timestamp"]; ok {
		ts, _ = time.Parse(time.RFC3339, fmt.Sprintf("%v", tsStr))
	}
	if ts.IsZero() {
		ts = time.Now()
	}

	// Run the pricing pipeline first so we can store the cost on the UsageLog.
	event := pricing.UsageEvent{
		TenantID:            tenantID,
		Provider:            provider,
		Model:               model,
		InputTokens:         inputTokens,
		OutputTokens:        outputTokens,
		CacheCreationTokens: cacheCreationTokens,
		CacheReadTokens:     cacheReadTokens,
		Timestamp:           ts,
		IdempotencyKey:      messageID,
		APIKeyRef:           keyID,
		APIUsageBilled:      apiUsageBilled,
	}
	result, err := w.pricingEngine.Process(ctx, event)
	if err != nil {
		return fmt.Errorf("pricing engine: %w", err)
	}

	// Write UsageLog record with the computed cost.
	usageLog := &models.UsageLog{
		TenantID:            tenantID,
		Provider:            provider,
		Model:               model,
		PromptTokens:        inputTokens,
		CompletionTokens:    outputTokens,
		CacheCreationTokens: cacheCreationTokens,
		CacheReadTokens:     cacheReadTokens,
		Cost:                result.FinalCost,
		RequestID:           messageID,
		KeyID:               keyID,
		APIKeyFingerprint:   apiKeyFingerprint,
		CreatedAt:           ts,
		APIUsageBilled:      apiUsageBilled,
	}
	if err := w.usageSvc.Create(ctx, usageLog); err != nil {
		// Ignore duplicate request_id (already processed)
		if isDuplicateError(err) {
			log.Printf("usageworker: duplicate message_id=%s, skipping usage log", messageID)
		} else {
			return fmt.Errorf("create usage log: %w", err)
		}
	}

	return nil
}

func isDuplicateError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	for _, substr := range []string{"duplicate key", "unique constraint", "23505"} {
		if len(s) >= len(substr) {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
		}
	}
	return false
}
