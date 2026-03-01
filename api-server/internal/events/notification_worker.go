package events

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/smtp"
	"os"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"

	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

const notifConsumerGroup = "tokengate:notification:workers"
const notifConsumerName = "notif-worker-1"
const debounceTTL = 1 * time.Minute

// NotificationWorker consumes notification events from Redis Streams and dispatches alerts.
type NotificationWorker struct {
	rdb *redis.Client
	db  *gorm.DB
}

// NewNotificationWorker creates a new NotificationWorker.
func NewNotificationWorker(rdb *redis.Client, db *gorm.DB) *NotificationWorker {
	return &NotificationWorker{rdb: rdb, db: db}
}

// Run starts the Redis Streams consumer loop. It blocks until ctx is cancelled.
func (w *NotificationWorker) Run(ctx context.Context) {
	err := w.rdb.XGroupCreateMkStream(ctx, notificationStreamName, notifConsumerGroup, "$").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		slog.Error("notifworker_xgroup_create_failed", "error", err)
	}

	slog.Info("notifworker_started", "stream", notificationStreamName, "group", notifConsumerGroup)

	for {
		select {
		case <-ctx.Done():
			slog.Info("notifworker_stopping", "reason", "context_cancelled")
			return
		default:
		}

		streams, err := w.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    notifConsumerGroup,
			Consumer: notifConsumerName,
			Streams:  []string{notificationStreamName, ">"},
			Count:    10,
			Block:    5 * time.Second,
		}).Result()
		if err != nil {
			if err == redis.Nil || err.Error() == "redis: nil" {
				continue
			}
			if ctx.Err() != nil {
				return
			}
			slog.Error("notifworker_xreadgroup_error", "error", err)
			time.Sleep(time.Second)
			continue
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				if err := w.processMessage(ctx, msg); err != nil {
					slog.Error("notifworker_process_failed", "msg_id", msg.ID, "error", err)
					continue
				}
				if err := w.rdb.XAck(ctx, notificationStreamName, notifConsumerGroup, msg.ID).Err(); err != nil {
					slog.Error("notifworker_xack_failed", "msg_id", msg.ID, "error", err)
				}
			}
		}
	}
}

func (w *NotificationWorker) processMessage(ctx context.Context, msg redis.XMessage) error {
	v := msg.Values

	tenantID64, err := strconv.ParseUint(fmt.Sprintf("%v", v["tenant_id"]), 10, 64)
	if err != nil {
		return fmt.Errorf("parse tenant_id: %w", err)
	}
	tenantID := uint(tenantID64)
	eventType := fmt.Sprintf("%v", v["event_type"])
	keyID := fmt.Sprintf("%v", v["key_id"])
	provider := fmt.Sprintf("%v", v["provider"])
	model := fmt.Sprintf("%v", v["model"])
	details := fmt.Sprintf("%v", v["details"])

	// Debounce: skip if already sent recently for this tenant+event+key+provider+model
	dedupKey := fmt.Sprintf("notif:dedup:%d:%s:%s:%s:%s", tenantID, eventType, keyID, provider, model)
	set, err := w.rdb.SetNX(ctx, dedupKey, "1", debounceTTL).Result()
	if err != nil {
		slog.Error("notifworker_dedup_check_failed", "error", err)
		// Continue anyway — better to send a duplicate than miss an alert
	} else if !set {
		slog.Debug("notification_debounced",
			"tenant_id", tenantID, "event_type", eventType,
		)
		return nil
	}

	// Load enabled channels matching this event type
	var channels []models.NotificationChannel
	if err := w.db.WithContext(ctx).
		Where("tenant_id = ? AND enabled = ? AND ? = ANY(event_types)", tenantID, true, eventType).
		Find(&channels).Error; err != nil {
		return fmt.Errorf("load channels: %w", err)
	}

	if len(channels) == 0 {
		return nil
	}

	payload := notificationPayload{
		TenantID:  tenantID,
		EventType: eventType,
		KeyID:     keyID,
		Provider:  provider,
		Model:     model,
		Details:   details,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	for _, ch := range channels {
		var dispatchErr error
		switch ch.ChannelType {
		case "email":
			dispatchErr = w.sendEmail(ch, payload)
		case "slack":
			dispatchErr = w.sendSlack(ch, payload)
		case "webhook":
			dispatchErr = w.sendWebhook(ch, payload)
		default:
			slog.Warn("notifworker_unknown_channel_type", "channel_type", ch.ChannelType, "channel_id", ch.ID)
			continue
		}

		if dispatchErr != nil {
			slog.Error("notification_dispatch_failed",
				"tenant_id", tenantID, "channel_id", ch.ID,
				"channel_type", ch.ChannelType, "event_type", eventType,
				"error", dispatchErr,
			)
		} else {
			slog.Info("notification_dispatched",
				"tenant_id", tenantID, "channel_id", ch.ID,
				"channel_type", ch.ChannelType, "event_type", eventType,
			)
		}
	}

	return nil
}

type notificationPayload struct {
	TenantID  uint   `json:"tenant_id"`
	EventType string `json:"event_type"`
	KeyID     string `json:"key_id"`
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	Details   string `json:"details"`
	Timestamp string `json:"timestamp"`
}

// sendEmail dispatches a notification via SMTP.
func (w *NotificationWorker) sendEmail(ch models.NotificationChannel, p notificationPayload) error {
	var cfg struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal([]byte(ch.Config), &cfg); err != nil {
		return fmt.Errorf("parse email config: %w", err)
	}
	if cfg.Email == "" {
		return fmt.Errorf("email address not configured")
	}

	smtpHost := os.Getenv("SMTP_HOST")
	smtpPort := os.Getenv("SMTP_PORT")
	smtpUser := os.Getenv("SMTP_USER")
	smtpPass := os.Getenv("SMTP_PASS")
	smtpFrom := os.Getenv("SMTP_FROM")

	if smtpHost == "" || smtpPort == "" {
		return fmt.Errorf("SMTP not configured (SMTP_HOST/SMTP_PORT missing)")
	}
	if smtpFrom == "" {
		smtpFrom = smtpUser
	}

	subject := fmt.Sprintf("TokenGate Alert: %s", eventTypeLabel(p.EventType))
	body := fmt.Sprintf("Event: %s\nProvider: %s\nModel: %s\nAPI Key: %s\nDetails: %s\nTime: %s",
		eventTypeLabel(p.EventType), p.Provider, p.Model, p.KeyID, p.Details, p.Timestamp)

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		smtpFrom, cfg.Email, subject, body)

	addr := smtpHost + ":" + smtpPort
	var auth smtp.Auth
	if smtpUser != "" {
		auth = smtp.PlainAuth("", smtpUser, smtpPass, smtpHost)
	}

	return smtp.SendMail(addr, auth, smtpFrom, []string{cfg.Email}, []byte(msg))
}

// sendSlack dispatches a notification via Slack incoming webhook.
func (w *NotificationWorker) sendSlack(ch models.NotificationChannel, p notificationPayload) error {
	var cfg struct {
		SlackWebhookURL string `json:"slack_webhook_url"`
	}
	if err := json.Unmarshal([]byte(ch.Config), &cfg); err != nil {
		return fmt.Errorf("parse slack config: %w", err)
	}
	if cfg.SlackWebhookURL == "" {
		return fmt.Errorf("slack_webhook_url not configured")
	}

	text := fmt.Sprintf("*TokenGate Alert: %s*\n>Provider: %s\n>Model: %s\n>API Key: `%s`\n>Details: %s\n>Time: %s",
		eventTypeLabel(p.EventType), p.Provider, p.Model, p.KeyID, p.Details, p.Timestamp)

	payload, _ := json.Marshal(map[string]string{"text": text})

	resp, err := http.Post(cfg.SlackWebhookURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("slack webhook request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("slack webhook returned %d", resp.StatusCode)
	}
	return nil
}

// sendWebhook dispatches a notification via HTTP POST to a custom webhook URL.
func (w *NotificationWorker) sendWebhook(ch models.NotificationChannel, p notificationPayload) error {
	var cfg struct {
		WebhookURL    string `json:"webhook_url"`
		SigningSecret string `json:"signing_secret"`
	}
	if err := json.Unmarshal([]byte(ch.Config), &cfg); err != nil {
		return fmt.Errorf("parse webhook config: %w", err)
	}
	if cfg.WebhookURL == "" {
		return fmt.Errorf("webhook_url not configured")
	}

	body, _ := json.Marshal(p)

	req, err := http.NewRequest("POST", cfg.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if cfg.SigningSecret != "" {
		mac := hmac.New(sha256.New, []byte(cfg.SigningSecret))
		mac.Write(body)
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-TokenGate-Signature", sig)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}
	return nil
}

// SendTestNotification sends a test notification to a specific channel, bypassing debounce.
func (w *NotificationWorker) SendTestNotification(ctx context.Context, ch models.NotificationChannel) error {
	payload := notificationPayload{
		TenantID:  ch.TenantID,
		EventType: "test",
		KeyID:     "test-key",
		Provider:  "test-provider",
		Model:     "test-model",
		Details:   `{"message":"This is a test notification from TokenGate"}`,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	switch ch.ChannelType {
	case "email":
		return w.sendEmail(ch, payload)
	case "slack":
		return w.sendSlack(ch, payload)
	case "webhook":
		return w.sendWebhook(ch, payload)
	default:
		return fmt.Errorf("unknown channel type: %s", ch.ChannelType)
	}
}

func eventTypeLabel(et string) string {
	switch et {
	case models.EventBudgetBlocked:
		return "Budget Blocked"
	case models.EventBudgetWarning:
		return "Budget Warning"
	case models.EventRateLimitExceeded:
		return "Rate Limit Exceeded"
	case "test":
		return "Test Notification"
	default:
		return et
	}
}
