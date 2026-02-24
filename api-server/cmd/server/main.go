package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // embed IANA timezone database for containers without system tzdata

	"github.com/go-redis/redis/v8"
	"github.com/joho/godotenv"

	"github.com/xiaoboyu/tokengate/api-server/internal/api"
	"github.com/xiaoboyu/tokengate/api-server/internal/config"
	"github.com/xiaoboyu/tokengate/api-server/internal/db"
	"github.com/xiaoboyu/tokengate/api-server/internal/events"
	"github.com/xiaoboyu/tokengate/api-server/internal/pricing"
	"github.com/xiaoboyu/tokengate/api-server/internal/proxy"
	"github.com/xiaoboyu/tokengate/api-server/internal/ratelimit"
	"github.com/xiaoboyu/tokengate/api-server/internal/services"
)

func main() {
	_ = godotenv.Load()

	confFile := "/app/conf/api-server-prod.yaml"
	if _, err := os.Stat(confFile); os.IsNotExist(err) {
		confFile = "./conf/api-server-prod.yaml"
	}
	log.Printf("loading config from: %s", confFile)

	cfg, err := config.LoadConfig(confFile)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	log.Printf("config loaded: env=%s host=%s port=%s", cfg.Environment, cfg.Server.Host, cfg.Server.Port)

	// PostgreSQL
	postgresDSN := cfg.Postgres.DSN
	if v := os.Getenv("POSTGRES_DB_URL"); v != "" {
		postgresDSN = v
	}
	postgresDB, err := db.InitPostgres(postgresDSN)
	if err != nil {
		log.Fatalf("postgres init err: %v", err)
	}
	log.Println("✓ PostgreSQL connected")

	// Redis
	var rdb *redis.Client
	if redisURL := os.Getenv("REDIS_URL"); redisURL != "" {
		opts, err := redis.ParseURL(redisURL)
		if err != nil {
			log.Fatalf("failed to parse REDIS_URL: %v", err)
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
		log.Fatalf("redis ping err: %v", err)
	}
	log.Println("✓ Redis connected")

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
		log.Fatalf("provider key service init err: %v", err)
	}
	log.Println("✓ Provider key service initialized")

	// Event queue (Redis Streams producer)
	eventQueue := events.NewEventQueue(rdb)

	// Usage worker (Redis Streams consumer)
	usageWorker := events.NewUsageWorker(rdb, pricingEngine, usageSvc)
	go usageWorker.Run(context.Background())
	log.Println("✓ Usage worker started")

	// Rate limiter
	rateLimiter := ratelimit.NewLimiter(postgresDB.GetDB(), rdb)

	// Proxy handler
	proxyHandler := proxy.NewProxyHandler(providerKeySvc, eventQueue, pricingEngine, rateLimiter)

	// API server
	apiServer := api.NewServer(cfg, postgresDB, apiKeySvc, usageSvc, pricingEngine, providerKeySvc, proxyHandler, rateLimiter)
	go func() {
		if err := apiServer.Run(); err != nil {
			log.Fatalf("server err: %v", err)
		}
	}()
	log.Printf("✓ Server started on %s:%s", cfg.Server.Host, cfg.Server.Port)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down...")
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := apiServer.Shutdown(shutCtx); err != nil {
		log.Printf("forced shutdown: %v", err)
	}
	log.Println("server exited")
}
