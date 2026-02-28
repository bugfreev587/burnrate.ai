package events

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
)

const streamName = "tokengate:usage:events"

// EventQueue is a Redis Streams producer.
type EventQueue struct {
	rdb *redis.Client
}

// NewEventQueue creates a new EventQueue.
func NewEventQueue(rdb *redis.Client) *EventQueue {
	return &EventQueue{rdb: rdb}
}

// UsageEventMsg is the payload published to the Redis stream.
type UsageEventMsg struct {
	TenantID            uint
	KeyID               string // api_keys.key_id — used for api_key-level budget tracking
	Provider            string
	Model               string
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
	MessageID           string // Anthropic message ID used as idempotency key
	ProviderKeyHint     string // masked provider API key e.g. "sk-ant-api03-9gq...aQAA"
	LatencyMs           int64
	Timestamp           time.Time
	APIUsageBilled      bool
}

// Publish XADD tokengate:usage:events * field value ...
// Fire-and-forget: errors are logged but not propagated to the caller.
func (q *EventQueue) Publish(ctx context.Context, msg UsageEventMsg) error {
	values := map[string]interface{}{
		"tenant_id":             strconv.FormatUint(uint64(msg.TenantID), 10),
		"key_id":                msg.KeyID,
		"provider":              msg.Provider,
		"model":                 msg.Model,
		"input_tokens":          strconv.FormatInt(msg.InputTokens, 10),
		"output_tokens":         strconv.FormatInt(msg.OutputTokens, 10),
		"cache_creation_tokens": strconv.FormatInt(msg.CacheCreationTokens, 10),
		"cache_read_tokens":     strconv.FormatInt(msg.CacheReadTokens, 10),
		"message_id":            msg.MessageID,
		"provider_key_hint":     msg.ProviderKeyHint,
		"latency_ms":            strconv.FormatInt(msg.LatencyMs, 10),
		"timestamp":             msg.Timestamp.UTC().Format(time.RFC3339),
		"api_usage_billed":      strconv.FormatBool(msg.APIUsageBilled),
	}
	err := q.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: streamName,
		Values: values,
	}).Err()
	if err != nil {
		return fmt.Errorf("eventqueue: XADD %s: %w", streamName, err)
	}
	return nil
}
