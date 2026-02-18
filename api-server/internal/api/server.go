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
	"github.com/xiaoboyu/burnrate-ai/api-server/internal/pricing"
	"github.com/xiaoboyu/burnrate-ai/api-server/internal/services"
)

type Server struct {
	cfg           *config.Config
	postgresDB    *db.PostgresDB
	apiKeySvc     *services.APIKeyService
	usageSvc      *services.UsageLogService
	pricingEngine *pricing.PricingEngine
	rbac          *middleware.RBACMiddleware
	router        *gin.Engine
	httpServer    *http.Server
}

func NewServer(
	cfg *config.Config,
	postgresDB *db.PostgresDB,
	apiKeySvc *services.APIKeyService,
	usageSvc *services.UsageLogService,
	pricingEngine *pricing.PricingEngine,
) *Server {
	if cfg.Environment == "production" || cfg.Environment == "prod" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	rbac := middleware.NewRBACMiddleware(postgresDB.GetDB())

	s := &Server{
		cfg:           cfg,
		postgresDB:    postgresDB,
		apiKeySvc:     apiKeySvc,
		usageSvc:      usageSvc,
		pricingEngine: pricingEngine,
		rbac:          rbac,
		router:        router,
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
	s.router.POST("/v1/auth/sync", s.handleAuthSync)

	// ─── API-key authenticated (agent → reports usage) ───────────────────────
	agent := s.router.Group("/v1/agent")
	agent.Use(apiKeyAuth)
	{
		agent.POST("/usage", s.handleReportUsage)
	}

	// ─── Viewer+ (any signed-in tenant member) ───────────────────────────────
	viewer := s.router.Group("/v1")
	viewer.Use(s.rbac.RequireUser(), s.rbac.RequireViewer())
	{
		viewer.GET("/usage", s.handleListUsage)
		viewer.GET("/usage/summary", s.handleUsageSummary)
		viewer.GET("/cost-ledger", s.handleListCostLedger)
		viewer.GET("/usage/forecast", s.handleUsageForecast)
	}

	// ─── Admin+ ──────────────────────────────────────────────────────────────
	admin := s.router.Group("/v1/admin")
	admin.Use(s.rbac.RequireUser(), s.rbac.RequireAdmin())
	{
		// User management
		admin.GET("/users", s.handleListUsers)
		admin.POST("/users/invite", s.handleInviteUser)
		admin.PATCH("/users/:user_id/role", s.handleUpdateUserRole)
		admin.PATCH("/users/:user_id/suspend", s.handleSuspendUser)
		admin.PATCH("/users/:user_id/unsuspend", s.handleUnsuspendUser)
		admin.DELETE("/users/:user_id", s.handleRemoveUser)

		// API key management
		admin.POST("/api_keys", s.handleCreateAPIKey)
		admin.GET("/api_keys", s.handleListAPIKeys)
		admin.DELETE("/api_keys/:key_id", s.handleRevokeAPIKey)

		// Provider key management
		admin.POST("/provider_keys", s.handleCreateProviderKey)
		admin.GET("/provider_keys", s.handleListProviderKeys)
		admin.DELETE("/provider_keys/:key_id", s.handleRevokeProviderKey)

		// Pricing administration
		pricingGroup := admin.Group("/pricing")
		{
			pricingGroup.GET("/providers", s.handleListProviders)
			pricingGroup.POST("/providers", s.handleCreateProvider)
			pricingGroup.GET("/models", s.handleListModels)
			pricingGroup.POST("/models", s.handleCreateModel)
			pricingGroup.GET("/model-pricing", s.handleListModelPricing)
			pricingGroup.POST("/model-pricing", s.handleCreateModelPricing)
			pricingGroup.GET("/markups", s.handleListMarkups)
			pricingGroup.POST("/markups", s.handleCreateMarkup)
			pricingGroup.DELETE("/markups/:markup_id", s.handleDeleteMarkup)
			pricingGroup.GET("/contracts", s.handleListContracts)
			pricingGroup.POST("/contracts", s.handleCreateContract)
		}

		// Budget management
		admin.GET("/budget", s.handleGetBudget)
		admin.PUT("/budget", s.handleUpsertBudget)
	}

	// ─── Owner only ──────────────────────────────────────────────────────────
	owner := s.router.Group("/v1/owner")
	owner.Use(s.rbac.RequireUser(), s.rbac.RequireOwner())
	{
		owner.POST("/users/:user_id/promote-admin", s.handlePromoteAdmin)
		owner.DELETE("/users/:user_id/demote-admin", s.handleDemoteAdmin)
		owner.POST("/transfer-ownership", s.handleTransferOwnership)
		owner.GET("/settings", s.handleGetTenantSettings)
		owner.PATCH("/settings", s.handleUpdateTenantSettings)
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
