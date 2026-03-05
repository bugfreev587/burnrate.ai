package api

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"

	"github.com/xiaoboyu/tokengate/api-server/internal/config"
	"github.com/xiaoboyu/tokengate/api-server/internal/db"
	"github.com/xiaoboyu/tokengate/api-server/internal/events"
	"github.com/xiaoboyu/tokengate/api-server/internal/middleware"
	"github.com/xiaoboyu/tokengate/api-server/internal/models"
	"github.com/xiaoboyu/tokengate/api-server/internal/pricing"
	"github.com/xiaoboyu/tokengate/api-server/internal/proxy"
	"github.com/xiaoboyu/tokengate/api-server/internal/ratelimit"
	"github.com/xiaoboyu/tokengate/api-server/internal/services"
)

type Server struct {
	cfg            *config.Config
	postgresDB     *db.PostgresDB
	rdb            *redis.Client
	apiKeySvc      *services.APIKeyService
	usageSvc       *services.UsageLogService
	pricingEngine  *pricing.PricingEngine
	providerKeySvc *services.ProviderKeyService
	proxyHandler   *proxy.ProxyHandler
	rateLimiter    *ratelimit.Limiter
	stripeSvc             *services.StripeService
	sandboxStripeSvc      *services.StripeService
	auditSvc              *services.AuditReportService
	auditLogSvc    *services.AuditLogService
	reportQueue    *events.ReportQueue
	notifWorker    *events.NotificationWorker
	rbac           *middleware.RBACMiddleware
	router         *gin.Engine
	httpServer     *http.Server
	clerkSecretKey   string
	internalSecret   string
	superAdminEmails map[string]bool
}

func NewServer(
	cfg *config.Config,
	postgresDB *db.PostgresDB,
	rdb *redis.Client,
	apiKeySvc *services.APIKeyService,
	usageSvc *services.UsageLogService,
	pricingEngine *pricing.PricingEngine,
	providerKeySvc *services.ProviderKeyService,
	proxyHandler *proxy.ProxyHandler,
	rateLimiter *ratelimit.Limiter,
	stripeSvc *services.StripeService,
	sandboxStripeSvc *services.StripeService,
	auditSvc *services.AuditReportService,
	auditLogSvc *services.AuditLogService,
	reportQueue *events.ReportQueue,
	notifWorker *events.NotificationWorker,
) *Server {
	if cfg.Environment == "production" || cfg.Environment == "prod" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	rbac := middleware.NewRBACMiddleware(postgresDB.GetDB())

	s := &Server{
		cfg:            cfg,
		postgresDB:     postgresDB,
		rdb:            rdb,
		apiKeySvc:      apiKeySvc,
		usageSvc:       usageSvc,
		pricingEngine:  pricingEngine,
		providerKeySvc: providerKeySvc,
		proxyHandler:   proxyHandler,
		rateLimiter:    rateLimiter,
		stripeSvc:        stripeSvc,
		sandboxStripeSvc: sandboxStripeSvc,
		auditSvc:         auditSvc,
		auditLogSvc:    auditLogSvc,
		reportQueue:    reportQueue,
		notifWorker:    notifWorker,
		rbac:           rbac,
		router:         router,
		clerkSecretKey:   os.Getenv("CLERK_SECRET_KEY"),
		internalSecret:   os.Getenv("INTERNAL_ADMIN_SECRET"),
		superAdminEmails: parseSuperAdminEmails(os.Getenv("SUPER_ADMIN_EMAILS")),
	}

	s.setupMiddleware()
	s.setupRoutes()
	return s
}

func (s *Server) setupMiddleware() {
	s.router.Use(DebugHeadersMiddleware()) // no-op unless DEBUG_HEADERS=true
	s.router.Use(gin.Recovery())
	s.router.Use(LoggerMiddleware())
	s.router.Use(CORSMiddleware(s.cfg.Server.CORSOrigins))
	s.router.Use(RateLimitMiddleware(s.cfg.Server.RateLimitPerMinute))
}

func (s *Server) setupRoutes() {
	apiKeyAuth := middleware.APIKeyMiddleware(s.apiKeySvc)
	// tenantAuth validates X-TokenGate-Key (or passes through when ENABLE_GW_VALIDATION=false).
	tenantAuth := middleware.TenantAuthMiddleware(s.apiKeySvc)

	// ─── Global health (Railway LB + backward compat) ────────────────────────
	s.router.GET("/health", s.handleHealth)
	s.router.GET("/v1/health", s.handleHealth)
	s.router.POST("/v1/auth/sync", s.handleAuthSync)

	// ─── Proxy routes — validated via X-TokenGate-Key ────────────────────────
	proxyGroup := s.router.Group("/v1")
	proxyGroup.Use(tenantAuth, middleware.PathProviderGuard())
	{
		proxyGroup.POST("/messages", s.proxyHandler.HandleProxy)
		proxyGroup.POST("/messages/count_tokens", s.proxyHandler.HandleCountTokens)
		proxyGroup.POST("/responses", s.proxyHandler.HandleResponses)
		proxyGroup.GET("/models", s.proxyHandler.HandleModels)
		proxyGroup.Any("/openai/*path", s.proxyHandler.HandleProxy)
		proxyGroup.Any("/gemini/*path", s.proxyHandler.HandleProxy)
		proxyGroup.Any("/bedrock/*path", s.proxyHandler.HandleProxy)
		proxyGroup.Any("/vertex/*path", s.proxyHandler.HandleProxy)
		proxyGroup.GET("/statusline", s.handleStatusLine)
	}

	// ─── API-key authenticated (agent → reports usage) ───────────────────────
	agent := s.router.Group("/v1/agent")
	agent.Use(apiKeyAuth)
	{
		agent.POST("/usage", s.handleReportUsage)
	}

	// ─── Projects (viewer+ for read, admin+ for mutations) ──────────────────
	projectViewer := s.router.Group("/v1/projects")
	projectViewer.Use(s.rbac.RequireUser(), s.rbac.RequireOrgRole(models.RoleViewer))
	{
		projectViewer.GET("", s.handleListProjects)
		projectViewer.GET("/:id", s.handleGetProject)
		projectViewer.GET("/:id/members", s.handleListProjectMembers)
	}
	projectAdmin := s.router.Group("/v1/projects")
	projectAdmin.Use(s.rbac.RequireUser(), s.rbac.RequireOrgRole(models.RoleAdmin))
	{
		projectAdmin.POST("", s.handleCreateProject)
		projectAdmin.PATCH("/:id", s.handleUpdateProject)
		projectAdmin.DELETE("/:id", s.handleDeleteProject)
		projectAdmin.POST("/:id/members", s.handleAddProjectMember)
		projectAdmin.PATCH("/:id/members/:user_id", s.handleUpdateProjectMemberRole)
		projectAdmin.DELETE("/:id/members/:user_id", s.handleRemoveProjectMember)
	}

	// ─── Viewer+ (any signed-in tenant member) ───────────────────────────────
	viewer := s.router.Group("/v1")
	viewer.Use(s.rbac.RequireUser(), s.rbac.RequireOrgRole(models.RoleViewer))
	{
		viewer.GET("/user/onboarding-hints", s.handleGetOnboardingHints)
		viewer.PATCH("/user/onboarding-hints", s.handleUpdateOnboardingHints)
		viewer.GET("/user/notifications", s.handleListUserNotifications)
		viewer.PATCH("/user/notifications/:id/read", s.handleMarkUserNotificationRead)
		viewer.DELETE("/user/notifications/:id", s.handleDeleteUserNotification)
		viewer.PATCH("/user/notifications/read-all", s.handleMarkAllUserNotificationsRead)
		viewer.GET("/user/notification-channels", s.handleListUserNotificationChannels)
		viewer.POST("/user/notification-channels", s.handleCreateUserNotificationChannel)
		viewer.PUT("/user/notification-channels/:id", s.handleUpdateUserNotificationChannel)
		viewer.DELETE("/user/notification-channels/:id", s.handleDeleteUserNotificationChannel)
		viewer.POST("/user/notification-channels/:id/test", s.handleTestUserNotificationChannel)
		viewer.POST("/user/invitations/:tenant_id/accept", s.handleAcceptInvitation)
		viewer.POST("/user/invitations/:tenant_id/deny", s.handleDenyInvitation)
		viewer.GET("/usage", s.handleListUsage)
		viewer.GET("/usage/summary", s.handleUsageSummary)
		viewer.GET("/cost-ledger", s.handleListCostLedger)
		viewer.GET("/usage/forecast", s.handleUsageForecast)
		viewer.GET("/usage/metrics", s.handleUsageMetrics)
		viewer.GET("/dashboard/config", s.handleDashboardConfig)
		viewer.GET("/dashboard/summary", s.handleDashboardSummary)
		viewer.GET("/dashboard/recent-requests", s.handleDashboardRecentRequests)
		viewer.GET("/budget", s.handleGetBudget)
		viewer.GET("/rate-limits", s.handleListRateLimits)
		viewer.GET("/audit/reports", s.handleListAuditReports)
		viewer.GET("/audit/reports/:id", s.handleGetAuditReport)
		viewer.GET("/audit/reports/:id/download", s.handleDownloadAuditReport)
	}

	// ─── Audit Admin (create/delete reports + audit logs, admin+) ───────────
	auditAdmin := s.router.Group("/v1/audit")
	auditAdmin.Use(s.rbac.RequireUser(), s.rbac.RequireOrgRole(models.RoleAdmin))
	{
		auditAdmin.POST("/reports", s.handleCreateAuditReport)
		auditAdmin.DELETE("/reports/:id", s.handleDeleteAuditReport)
	}
	auditLogsGroup := s.router.Group("/v1")
	auditLogsGroup.Use(s.rbac.RequireUser(), s.rbac.RequireOrgRole(models.RoleAdmin))
	{
		auditLogsGroup.GET("/audit-logs", s.handleListAuditLogs)
	}

	// ─── Editor+ (Management, Limits, Pricing Config) ───────────────────────
	admin := s.router.Group("/v1/admin")
	admin.Use(s.rbac.RequireUser(), s.rbac.RequireOrgRole(models.RoleEditor))
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

		// Budget management (read — write routes moved to limitsAdmin below)
		admin.GET("/budget", s.handleGetBudget)

		// Rate limit management (read — write routes moved to limitsAdmin below)
		admin.GET("/rate-limits", s.handleListRateLimits)

		// Notification channel management
		admin.GET("/notifications", s.handleListNotificationChannels)
		admin.POST("/notifications", s.handleCreateNotificationChannel)
		admin.PUT("/notifications/:id", s.handleUpdateNotificationChannel)
		admin.DELETE("/notifications/:id", s.handleDeleteNotificationChannel)
		admin.POST("/notifications/:id/test", s.handleTestNotificationChannel)
	}

	// ─── Limits Admin (budget + rate-limit mutations, admin+ only) ──────────
	limitsAdmin := s.router.Group("/v1/admin")
	limitsAdmin.Use(s.rbac.RequireUser(), s.rbac.RequireOrgRole(models.RoleAdmin))
	{
		limitsAdmin.PUT("/budget", s.handleUpsertBudget)
		limitsAdmin.DELETE("/budget/:budget_id", s.handleDeleteBudget)
		limitsAdmin.PUT("/rate-limits", s.handleUpsertRateLimit)
		limitsAdmin.DELETE("/rate-limits/:id", s.handleDeleteRateLimit)
	}

	// ─── Billing (viewer+ for read, admin+ for mutations) ───────────────────
	billingViewer := s.router.Group("/v1/billing")
	billingViewer.Use(s.rbac.RequireUser(), s.rbac.RequireOrgRole(models.RoleViewer))
	{
		billingViewer.GET("/status", s.handleBillingStatus)
		billingViewer.GET("/invoices", s.handleBillingInvoices)
	}

	billingAdmin := s.router.Group("/v1/billing")
	billingAdmin.Use(s.rbac.RequireUser(), s.rbac.RequireOrgRole(models.RoleAdmin))
	{
		billingAdmin.POST("/checkout", s.handleBillingCheckout)
		billingAdmin.POST("/checkout/verify", s.handleBillingCheckoutVerify)
		billingAdmin.POST("/portal", s.handleBillingPortal)
		billingAdmin.POST("/change-plan", s.handleBillingChangePlan)
		billingAdmin.POST("/downgrade", s.handleBillingDowngrade)
		billingAdmin.POST("/downgrade/cancel", s.handleBillingCancelDowngrade)
	}

	// Billing webhook (public — signature-verified in handler)
	s.router.POST("/v1/billing/webhook", s.handleBillingWebhook)
	// Sandbox billing webhook for super admin test transactions
	s.router.POST("/v1/billing/sandbox-webhook", s.handleBillingSandboxWebhook)

	// ─── Owner only ──────────────────────────────────────────────────────────
	owner := s.router.Group("/v1/owner")
	owner.Use(s.rbac.RequireUser(), s.rbac.RequireOrgRole(models.RoleOwner))
	{
		owner.POST("/users/:user_id/promote-admin", s.handlePromoteAdmin)
		owner.DELETE("/users/:user_id/demote-admin", s.handleDemoteAdmin)
		owner.POST("/transfer-ownership", s.handleTransferOwnership)
		owner.GET("/settings", s.handleGetTenantSettings)
		owner.PATCH("/settings", s.handleUpdateTenantSettings)
		owner.DELETE("/account", s.handleDeleteAccount)
		// Deprecated: use POST /v1/billing/checkout for plan changes via Stripe
		owner.PATCH("/plan", func(c *gin.Context) {
			c.JSON(http.StatusGone, gin.H{
				"error":   "deprecated",
				"message": "Plan changes are now managed via Stripe billing. Use POST /v1/billing/checkout to subscribe or POST /v1/billing/portal to manage your subscription.",
			})
		})
	}

	// ─── Internal / platform-admin ───────────────────────────────────────────
	internal := s.router.Group("/v1/internal")
	internal.Use(s.internalSecretMiddleware())
	{
		internal.PATCH("/tenants/:tenant_id/plan", s.handleAdminChangeTenantPlan)
	}

	// ─── Super Admin (email-based allowlist) ─────────────────────────────────
	superAdmin := s.router.Group("/v1/superadmin")
	superAdmin.Use(s.superAdminMiddleware())
	{
		superAdmin.GET("/whoami", s.handleSuperAdminWhoami)
		superAdmin.GET("/stats", s.handleSuperAdminStats)
		superAdmin.GET("/tenants", s.handleListAllTenants)
		superAdmin.GET("/tenants/:tenant_id", s.handleGetTenantDetail)
		superAdmin.PATCH("/tenants/:tenant_id/plan", s.handleSuperAdminChangePlan)
		superAdmin.PATCH("/tenants/:tenant_id/status", s.handleSuperAdminUpdateTenantStatus)
	}

	// ─── NoRoute: helpful error for bare /v1/… without tenant prefix ─────────
	s.router.NoRoute(s.handleNoRoute)
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
		if subtle.ConstantTimeCompare([]byte(c.GetHeader("X-Internal-Secret")), []byte(s.internalSecret)) != 1 {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid internal secret"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// Router returns the underlying http.Handler for use in tests.
func (s *Server) Router() http.Handler {
	return s.router
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

// parseSuperAdminEmails splits a comma-separated list of emails into a set.
func parseSuperAdminEmails(raw string) map[string]bool {
	m := make(map[string]bool)
	for _, e := range strings.Split(raw, ",") {
		e = strings.TrimSpace(strings.ToLower(e))
		if e != "" {
			m[e] = true
		}
	}
	return m
}

// superAdminMiddleware authenticates super-admin requests by checking the
// caller's email (looked up via X-User-ID) against the SUPER_ADMIN_EMAILS
// allowlist. Does NOT require X-Tenant-Id.
func (s *Server) superAdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if len(s.superAdminEmails) == 0 {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "super admin API not configured"})
			c.Abort()
			return
		}

		userID := c.GetHeader("X-User-ID")
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			c.Abort()
			return
		}

		var user models.User
		if err := s.postgresDB.GetDB().Where("id = ?", userID).First(&user).Error; err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
			c.Abort()
			return
		}

		if !s.superAdminEmails[strings.ToLower(user.Email)] {
			slog.Warn("super_admin_denied", "user_id", userID, "email", user.Email)
			c.JSON(http.StatusForbidden, gin.H{"error": "not a super admin"})
			c.Abort()
			return
		}

		c.Set("super_admin_user", &user)
		c.Next()
	}
}

// stripeServiceForContext returns the sandbox StripeService if the requesting user
// is a super admin and sandbox Stripe is configured; otherwise returns the production service.
// This allows super admin users to use Stripe test mode for billing operations.
func (s *Server) stripeServiceForContext(c *gin.Context) *services.StripeService {
	if s.sandboxStripeSvc != nil && s.sandboxStripeSvc.IsConfigured() {
		user, ok := middleware.GetUserFromContext(c)
		if ok && s.superAdminEmails[strings.ToLower(user.Email)] {
			return s.sandboxStripeSvc
		}
	}
	return s.stripeSvc
}
