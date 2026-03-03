package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata" // embed IANA timezone database for containers without system tzdata

	"github.com/go-redis/redis/v8"
	"github.com/joho/godotenv"

	"github.com/xiaoboyu/tokengate/api-server/internal/api"
	"github.com/xiaoboyu/tokengate/api-server/internal/config"
	"github.com/xiaoboyu/tokengate/api-server/internal/db"
	"github.com/xiaoboyu/tokengate/api-server/internal/events"
	"github.com/xiaoboyu/tokengate/api-server/internal/logging"
	"github.com/xiaoboyu/tokengate/api-server/internal/pricing"
	"github.com/xiaoboyu/tokengate/api-server/internal/proxy"
	"github.com/xiaoboyu/tokengate/api-server/internal/ratelimit"
	"github.com/xiaoboyu/tokengate/api-server/internal/services"
)

func main() {
	_ = godotenv.Load()

	// Initialize structured logging (stdout JSON + optional Better Stack).
	// Must be called before any other logging so the bridge captures everything.
	flush := logging.Init(os.Getenv("BETTERSTACK_SOURCE_TOKEN"))
	defer flush()

	env := strings.ToLower(os.Getenv("ENVIRONMENT"))
	confFile := ""
	switch env {
	case "staging":
		confFile = "/app/conf/api-server-staging.yaml"
	case "test":
		confFile = "/app/conf/api-server-test.yaml"
	default:
		confFile = "/app/conf/api-server-prod.yaml"
	}
	if _, err := os.Stat(confFile); os.IsNotExist(err) {
		switch env {
		case "staging":
			confFile = "./conf/api-server-staging.yaml"
		case "test":
			confFile = "./conf/api-server-test.yaml"
		default:
			confFile = "./conf/api-server-prod.yaml"
		}
	}
	slog.Info("loading config", "file", confFile)

	cfg, err := config.LoadConfig(confFile)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}
	slog.Info("config loaded", "env", cfg.Environment, "host", cfg.Server.Host, "port", cfg.Server.Port)

	// ── Production startup validations ───────────────────────────────────────
	if cfg.Environment == "production" || cfg.Environment == "prod" {
		if strings.EqualFold(os.Getenv("ENABLE_GW_VALIDATION"), "false") {
			slog.Error("ENABLE_GW_VALIDATION=false is not allowed in production")
			os.Exit(1)
		}
		pepper := cfg.Security.APIKeyPepper
		if pepper == "change-me-in-production" || len(pepper) < 16 {
			slog.Error("API_KEY_PEPPER must be at least 16 characters and not the default value in production")
			os.Exit(1)
		}
	}

	// PostgreSQL
	postgresDSN := cfg.Postgres.DSN
	if v := os.Getenv("POSTGRES_DB_URL"); v != "" {
		postgresDSN = v
	}
	postgresDB, err := db.InitPostgres(postgresDSN)
	if err != nil {
		slog.Error("postgres init failed", "error", err)
		os.Exit(1)
	}
	slog.Info("PostgreSQL connected")

	// Redis
	var rdb *redis.Client
	if redisURL := os.Getenv("REDIS_URL"); redisURL != "" {
		opts, err := redis.ParseURL(redisURL)
		if err != nil {
			slog.Error("failed to parse REDIS_URL", "error", err)
			os.Exit(1)
		}
		rdb = redis.NewClient(opts)
	} else {
		rdb = redis.NewClient(&redis.Options{
			Addr:     cfg.Redis.Addr,
			Password: cfg.Redis.Password,
			DB:       cfg.Redis.DB,
		})
	}
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		slog.Error("redis ping failed", "error", err)
		os.Exit(1)
	}
	slog.Info("Redis connected")

	// Services
	apiKeySvc := services.NewAPIKeyService(
		postgresDB.GetDB(),
		[]byte(cfg.Security.APIKeyPepper),
		rdb,
		time.Duration(cfg.Security.APIKeyCacheTTLSeconds)*time.Second,
	)
	usageSvc := services.NewUsageLogService(postgresDB.GetDB())

	// Pricing engine
	pricingEngine := pricing.NewPricingEngine(postgresDB.GetDB(), rdb)

	// Provider key service (requires PROVIDER_KEY_ENCRYPTION_KEY)
	providerKeySvc, err := services.NewProviderKeyService(postgresDB.GetDB(), cfg.Security.ProviderKeyEncryptionKey, rdb)
	if err != nil {
		slog.Error("provider key service init failed", "error", err)
		os.Exit(1)
	}
	slog.Info("Provider key service initialized")

	// Event queue (Redis Streams producer)
	eventQueue := events.NewEventQueue(rdb)

	// Usage worker (Redis Streams consumer)
	usageWorker := events.NewUsageWorker(rdb, pricingEngine, usageSvc)
	go usageWorker.Run(context.Background())
	slog.Info("Usage worker started")

	// Object store (Cloudflare R2) — optional
	var objStore *services.ObjectStore
	if cfg.R2.Endpoint != "" {
		objStore = services.NewObjectStore(cfg.R2.Endpoint, cfg.R2.AccessKeyID, cfg.R2.AccessKeySecret, cfg.R2.BucketName)
		slog.Info("R2 object store configured", "endpoint", cfg.R2.Endpoint, "bucket", cfg.R2.BucketName)
	}

	// Audit report service + queue + worker
	auditSvc := services.NewAuditReportService(postgresDB.GetDB(), objStore)
	auditLogSvc := services.NewAuditLogService(postgresDB.GetDB())
	reportQueue := events.NewReportQueue(rdb)
	reportWorker := events.NewReportWorker(rdb, postgresDB.GetDB(), auditSvc)
	go reportWorker.Run(context.Background())
	slog.Info("Report worker started")

	// Rate limiter
	rateLimiter := ratelimit.NewLimiter(postgresDB.GetDB(), rdb)

	// Notification queue + worker
	notifQueue := events.NewNotificationQueue(rdb)
	notifWorker := events.NewNotificationWorker(rdb, postgresDB.GetDB())
	go notifWorker.Run(context.Background())
	slog.Info("Notification worker started")

	// Wire notification queue into pricing engine and rate limiter
	pricingEngine.SetNotificationQueue(events.NewPricingNotificationAdapter(notifQueue))
	rateLimiter.SetNotificationQueue(events.NewRateLimitNotificationAdapter(notifQueue))

	// Stripe billing service
	stripeSvc := services.NewStripeService(postgresDB.GetDB(), cfg.Stripe)
	if stripeSvc.IsConfigured() {
		slog.Info("Stripe billing configured")
		if !stripeSvc.IsWebhookConfigured() {
			slog.Warn("Stripe is configured but STRIPE_WEBHOOK_SECRET is missing — webhooks will not be verified")
		}
	} else {
		slog.Warn("Stripe not configured — billing endpoints will return 503/empty")
	}

	// Gateway event service (records blocked requests: rate limits, budget exceeded)
	gatewayEventSvc := services.NewGatewayEventService(postgresDB.GetDB())

	// Proxy handler
	proxyHandler := proxy.NewProxyHandler(providerKeySvc, eventQueue, pricingEngine, rateLimiter, gatewayEventSvc)

	// API server
	apiServer := api.NewServer(cfg, postgresDB, rdb, apiKeySvc, usageSvc, pricingEngine, providerKeySvc, proxyHandler, rateLimiter, stripeSvc, auditSvc, auditLogSvc, reportQueue, notifWorker)
	go func() {
		if err := apiServer.Run(); err != nil {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()
	slog.Info("Server started", "host", cfg.Server.Host, "port", cfg.Server.Port)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down...")
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := apiServer.Shutdown(shutCtx); err != nil {
		slog.Error("forced shutdown", "error", err)
	}
	slog.Info("server exited")
}
