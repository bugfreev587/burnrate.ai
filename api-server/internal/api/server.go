package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/xiaoboyu/tokengate/api-server/internal/config"
	"github.com/xiaoboyu/tokengate/api-server/internal/db"
	"github.com/xiaoboyu/tokengate/api-server/internal/middleware"
	"github.com/xiaoboyu/tokengate/api-server/internal/pricing"
	"github.com/xiaoboyu/tokengate/api-server/internal/proxy"
	"github.com/xiaoboyu/tokengate/api-server/internal/services"
)

type Server struct {
	cfg              *config.Config
	postgresDB       *db.PostgresDB
	apiKeySvc        *services.APIKeyService
	usageSvc         *services.UsageLogService
	pricingEngine    *pricing.PricingEngine
	providerKeySvc   *services.ProviderKeyService
	proxyHandler     *proxy.ProxyHandler
	rbac             *middleware.RBACMiddleware
	router           *gin.Engine
	httpServer       *http.Server
	clerkSecretKey   string
	internalSecret   string
}

func NewServer(
	cfg *config.Config,
	postgresDB *db.PostgresDB,
	apiKeySvc *services.APIKeyService,
	usageSvc *services.UsageLogService,
	pricingEngine *pricing.PricingEngine,
	providerKeySvc *services.ProviderKeyService,
	proxyHandler *proxy.ProxyHandler,
) *Server {
	if cfg.Environment == "production" || cfg.Environment == "prod" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	rbac := middleware.NewRBACMiddleware(postgresDB.GetDB())

	s := &Server{
		cfg:            cfg,
		postgresDB:     postgresDB,
		apiKeySvc:      apiKeySvc,
		usageSvc:       usageSvc,
		pricingEngine:  pricingEngine,
		providerKeySvc: providerKeySvc,
		proxyHandler:   proxyHandler,
		rbac:           rbac,
		router:         router,
		clerkSecretKey: os.Getenv("CLERK_SECRET_KEY"),
		internalSecret: os.Getenv("INTERNAL_ADMIN_SECRET"),
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

	// ─── API-key authenticated: Anthropic proxy (for Claude Code agents) ────
	s.router.POST("/v1/messages", apiKeyAuth, s.proxyHandler.HandleProxy)
	s.router.GET("/v1/models", apiKeyAuth, s.proxyHandler.HandleModels)

	// ─── Multi-provider proxy routes (all HTTP methods) ──────────────────────
	s.router.Any("/v1/openai/*path", apiKeyAuth, s.proxyHandler.HandleProxy)
	s.router.Any("/v1/gemini/*path", apiKeyAuth, s.proxyHandler.HandleProxy)
	s.router.Any("/v1/bedrock/*path", apiKeyAuth, s.proxyHandler.HandleProxy)
	s.router.Any("/v1/vertex/*path", apiKeyAuth, s.proxyHandler.HandleProxy)

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
		admin.PUT("/provider_keys/:key_id/activate", s.handleActivateProviderKey)
		admin.POST("/provider_keys/:key_id/rotate", s.handleRotateProviderKey)

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
			pricingGroup.GET("/catalog", s.handleGetPricingCatalog)
		}

		// Pricing configs (per-key overrides)
		admin.GET("/pricing-configs", s.handleListPricingConfigs)
		admin.POST("/pricing-configs", s.handleCreatePricingConfig)
		admin.DELETE("/pricing-configs/:config_id", s.handleDeletePricingConfig)
		admin.POST("/pricing-configs/:config_id/rates", s.handleAddConfigRate)
		admin.DELETE("/pricing-configs/:config_id/rates/:rate_id", s.handleDeleteConfigRate)
		admin.PUT("/pricing-configs/:config_id/assign", s.handleAssignPricingConfig)
		admin.DELETE("/pricing-configs/:config_id/assign", s.handleUnassignPricingConfig)

		// Budget management
		admin.GET("/budget", s.handleGetBudget)
		admin.PUT("/budget", s.handleUpsertBudget)
		admin.DELETE("/budget/:budget_id", s.handleDeleteBudget)
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
		owner.PATCH("/plan", s.handleChangePlan)
	}

	// ─── Internal / platform-admin ───────────────────────────────────────────
	internal := s.router.Group("/v1/internal")
	internal.Use(s.internalSecretMiddleware())
	{
		internal.PATCH("/tenants/:tenant_id/plan", s.handleAdminChangeTenantPlan)
	}
}

// internalSecretMiddleware rejects requests that don't carry the correct
// X-Internal-Secret header. Used to protect platform-operator endpoints.
func (s *Server) internalSecretMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.internalSecret == "" {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "internal API not configured"})
			c.Abort()
			return
		}
		if c.GetHeader("X-Internal-Secret") != s.internalSecret {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid internal secret"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func (s *Server) Run() error {
	addr := fmt.Sprintf("%s:%s", s.cfg.Server.Host, s.cfg.Server.Port)
	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 300 * time.Second,
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
