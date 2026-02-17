package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/xiaoboyu/burnrate-ai/api-server/internal/config"
	"github.com/xiaoboyu/burnrate-ai/api-server/internal/db"
	"github.com/xiaoboyu/burnrate-ai/api-server/internal/middleware"
	"github.com/xiaoboyu/burnrate-ai/api-server/internal/services"
)

type Server struct {
	cfg       *config.Config
	postgresDB *db.PostgresDB
	apiKeySvc *services.APIKeyService
	usageSvc  *services.UsageLogService
	rbac      *middleware.RBACMiddleware
	router    *gin.Engine
	httpServer *http.Server
}

func NewServer(
	cfg *config.Config,
	postgresDB *db.PostgresDB,
	apiKeySvc *services.APIKeyService,
	usageSvc *services.UsageLogService,
) *Server {
	if cfg.Environment == "production" || cfg.Environment == "prod" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	rbac := middleware.NewRBACMiddleware(postgresDB.GetDB())

	s := &Server{
		cfg:        cfg,
		postgresDB: postgresDB,
		apiKeySvc:  apiKeySvc,
		usageSvc:   usageSvc,
		rbac:       rbac,
		router:     router,
	}

	s.setupMiddleware()
	s.setupRoutes()
	return s
}

func (s *Server) setupMiddleware() {
	s.router.Use(gin.Recovery())
	s.router.Use(LoggerMiddleware())
	s.router.Use(CORSMiddleware(s.cfg.Server.CORSOrigins))
	s.router.Use(RateLimitMiddleware(s.cfg.Server.RateLimitPerMinute))
}

func (s *Server) setupRoutes() {
	apiKeyAuth := middleware.APIKeyMiddleware(s.apiKeySvc)

	// ─── Public ────────────────────────────────────────────────────────────────
	s.router.GET("/v1/health", s.handleHealth)

	// Auth sync (Clerk → backend)
	s.router.POST("/v1/auth/sync", s.handleAuthSync)

	// ─── API-key authenticated (machine-to-machine) ─────────────────────────
	// e.g. claude-code agent reports usage
	agent := s.router.Group("/v1/agent")
	agent.Use(apiKeyAuth)
	{
		agent.POST("/usage", s.handleReportUsage)
	}

	// ─── Viewer+ (dashboard users) ──────────────────────────────────────────
	viewer := s.router.Group("/v1")
	viewer.Use(s.rbac.RequireUser(), s.rbac.RequireViewer())
	{
		viewer.GET("/usage", s.handleListUsage)
		viewer.GET("/usage/summary", s.handleUsageSummary)
	}

	// ─── Admin+ ─────────────────────────────────────────────────────────────
	admin := s.router.Group("/v1/admin")
	admin.Use(s.rbac.RequireUser(), s.rbac.RequireAdmin())
	{
		// API key management
		admin.POST("/api_keys", s.handleCreateAPIKey)
		admin.GET("/api_keys", s.handleListAPIKeys)
		admin.DELETE("/api_keys/:key_id", s.handleRevokeAPIKey)

		// Provider keys management
		admin.POST("/provider_keys", s.handleCreateProviderKey)
		admin.GET("/provider_keys", s.handleListProviderKeys)
		admin.DELETE("/provider_keys/:key_id", s.handleRevokeProviderKey)

		// Users
		admin.GET("/users", s.handleListUsers)
		admin.PATCH("/users/:user_id/role", s.handleUpdateUserRole)
		admin.PATCH("/users/:user_id/suspend", s.handleSuspendUser)
		admin.PATCH("/users/:user_id/unsuspend", s.handleUnsuspendUser)
	}
}

func (s *Server) Run() error {
	addr := fmt.Sprintf("%s:%s", s.cfg.Server.Host, s.cfg.Server.Port)
	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}
