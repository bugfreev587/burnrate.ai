package api

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

// ── Whoami ───────────────────────────────────────────────────────────────────

func (s *Server) handleSuperAdminWhoami(c *gin.Context) {
	user := c.MustGet("super_admin_user").(*models.User)
	c.JSON(http.StatusOK, gin.H{
		"is_super_admin": true,
		"user_id":        user.ID,
		"email":          user.Email,
	})
}

// ── Platform stats ───────────────────────────────────────────────────────────

func (s *Server) handleSuperAdminStats(c *gin.Context) {
	db := s.postgresDB.GetDB()

	// Tenants by plan
	type planCount struct {
		Plan  string
		Count int64
	}
	var planCounts []planCount
	db.Model(&models.Tenant{}).Select("plan, COUNT(*) as count").Group("plan").Scan(&planCounts)

	tenantsByPlan := map[string]int64{"free": 0, "pro": 0, "team": 0, "business": 0}
	var totalTenants int64
	for _, pc := range planCounts {
		tenantsByPlan[pc.Plan] = pc.Count
		totalTenants += pc.Count
	}

	// Active users
	var totalUsers int64
	db.Model(&models.User{}).Where("status = ?", "active").Count(&totalUsers)

	// Active API keys
	var totalAPIKeys int64
	db.Model(&models.APIKey{}).Where("revoked = false").Count(&totalAPIKeys)

	// 30-day usage
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30)
	var usageCount30d int64
	var totalCost30d decimal.Decimal
	row := db.Model(&models.UsageLog{}).
		Where("created_at >= ?", thirtyDaysAgo).
		Select("COUNT(*), COALESCE(SUM(cost), 0)").
		Row()
	_ = row.Scan(&usageCount30d, &totalCost30d)

	// Revenue from Stripe (total paid to TokenGate)
	totalRevenueCents, _, _ := s.stripeSvc.GetPlatformRevenue(c.Request.Context())

	c.JSON(http.StatusOK, gin.H{
		"total_tenants":    totalTenants,
		"tenants_by_plan":  tenantsByPlan,
		"total_users":      totalUsers,
		"total_api_keys":   totalAPIKeys,
		"usage_count_30d":  usageCount30d,
		"total_cost_30d":   totalCost30d,
		"total_revenue":    float64(totalRevenueCents) / 100.0,
	})
}

// ── List tenants ─────────────────────────────────────────────────────────────

type tenantListItem struct {
	ID           uint      `json:"id"`
	Name         string    `json:"name"`
	Plan         string    `json:"plan"`
	Status       string    `json:"status"`
	BillingEmail string    `json:"billing_email"`
	MemberCount  int64     `json:"member_count"`
	APIKeyCount  int64     `json:"api_key_count"`
	TotalPaid    float64   `json:"total_paid"`
	CreatedAt    time.Time `json:"created_at"`
}

func (s *Server) handleListAllTenants(c *gin.Context) {
	db := s.postgresDB.GetDB()

	search := c.Query("search")
	planFilter := c.Query("plan")
	statusFilter := c.Query("status")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "25"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 25
	}

	query := db.Model(&models.Tenant{})

	if search != "" {
		like := "%" + search + "%"
		query = query.Where("name ILIKE ? OR billing_email ILIKE ?", like, like)
	}
	if planFilter != "" {
		query = query.Where("plan = ?", planFilter)
	}
	if statusFilter != "" {
		query = query.Where("status = ?", statusFilter)
	}

	var total int64
	query.Count(&total)

	var tenants []models.Tenant
	query.Order("created_at DESC").
		Offset((page - 1) * perPage).
		Limit(perPage).
		Find(&tenants)

	// Fetch per-customer revenue from Stripe (single paginated call)
	_, perCustomerRevenue, _ := s.stripeSvc.GetPlatformRevenue(c.Request.Context())

	items := make([]tenantListItem, 0, len(tenants))
	for _, t := range tenants {
		var memberCount int64
		db.Model(&models.TenantMembership{}).Where("tenant_id = ?", t.ID).Count(&memberCount)

		var apiKeyCount int64
		db.Model(&models.APIKey{}).Where("tenant_id = ? AND revoked = false", t.ID).Count(&apiKeyCount)

		var totalPaidCents int64
		if t.StripeCustomerID != "" {
			totalPaidCents = perCustomerRevenue[t.StripeCustomerID]
		}

		items = append(items, tenantListItem{
			ID:           t.ID,
			Name:         t.Name,
			Plan:         t.Plan,
			Status:       t.Status,
			BillingEmail: t.BillingEmail,
			MemberCount:  memberCount,
			APIKeyCount:  apiKeyCount,
			TotalPaid:    float64(totalPaidCents) / 100.0,
			CreatedAt:    t.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"tenants":    items,
		"total":      total,
		"page":       page,
		"per_page":   perPage,
		"total_pages": int64(math.Ceil(float64(total) / float64(perPage))),
	})
}

// ── Tenant detail ────────────────────────────────────────────────────────────

type tenantMember struct {
	UserID  string `json:"user_id"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	OrgRole string `json:"org_role"`
	Status  string `json:"status"`
}

func (s *Server) handleGetTenantDetail(c *gin.Context) {
	db := s.postgresDB.GetDB()

	var tenantID uint
	if _, err := fmt.Sscanf(c.Param("tenant_id"), "%d", &tenantID); err != nil || tenantID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tenant_id"})
		return
	}

	var tenant models.Tenant
	if err := db.First(&tenant, tenantID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tenant not found"})
		return
	}

	// Members
	type memberRow struct {
		UserID  string
		Email   string
		Name    string
		OrgRole string
		Status  string
	}
	var memberRows []memberRow
	db.Table("tenant_memberships").
		Select("tenant_memberships.user_id, users.email, users.name, tenant_memberships.org_role, tenant_memberships.status").
		Joins("JOIN users ON users.id = tenant_memberships.user_id").
		Where("tenant_memberships.tenant_id = ?", tenantID).
		Scan(&memberRows)

	members := make([]tenantMember, 0, len(memberRows))
	for _, r := range memberRows {
		members = append(members, tenantMember{
			UserID:  r.UserID,
			Email:   r.Email,
			Name:    r.Name,
			OrgRole: r.OrgRole,
			Status:  r.Status,
		})
	}

	// Stats
	var apiKeyCount int64
	db.Model(&models.APIKey{}).Where("tenant_id = ? AND revoked = false", tenantID).Count(&apiKeyCount)

	var providerKeyCount int64
	db.Model(&models.ProviderKey{}).Where("tenant_id = ? AND revoked = false", tenantID).Count(&providerKeyCount)

	var projectCount int64
	db.Model(&models.Project{}).Where("tenant_id = ? AND status = ?", tenantID, models.ProjectStatusActive).Count(&projectCount)

	thirtyDaysAgo := time.Now().AddDate(0, 0, -30)
	var usageCount30d int64
	var totalCost30d decimal.Decimal
	row := db.Model(&models.UsageLog{}).
		Where("tenant_id = ? AND created_at >= ?", tenantID, thirtyDaysAgo).
		Select("COUNT(*), COALESCE(SUM(cost), 0)").
		Row()
	_ = row.Scan(&usageCount30d, &totalCost30d)

	// Revenue paid to TokenGate (from Stripe invoices)
	var totalPaidCents int64
	if tenant.StripeCustomerID != "" {
		invoices, err := s.stripeSvc.ListInvoices(c.Request.Context(), tenantID, 100)
		if err == nil {
			for _, inv := range invoices {
				if inv.Status == "paid" {
					totalPaidCents += inv.AmountPaid
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"tenant": gin.H{
			"id":                     tenant.ID,
			"name":                   tenant.Name,
			"plan":                   tenant.Plan,
			"status":                 tenant.Status,
			"billing_email":          tenant.BillingEmail,
			"stripe_customer_id":     tenant.StripeCustomerID,
			"stripe_subscription_id": tenant.StripeSubscriptionID,
			"plan_status":            tenant.PlanStatus,
			"current_period_end":     tenant.CurrentPeriodEnd,
			"pending_plan":           tenant.PendingPlan,
			"plan_effective_at":      tenant.PlanEffectiveAt,
			"created_at":             tenant.CreatedAt,
		},
		"plan_limits":        models.GetPlanLimits(tenant.Plan),
		"members":            members,
		"api_key_count":      apiKeyCount,
		"provider_key_count": providerKeyCount,
		"project_count":      projectCount,
		"usage_count_30d":    usageCount30d,
		"total_cost_30d":     totalCost30d,
		"total_paid":         float64(totalPaidCents) / 100.0,
	})
}

// ── Change plan ──────────────────────────────────────────────────────────────

func (s *Server) handleSuperAdminChangePlan(c *gin.Context) {
	var req changePlanReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var tenantID uint
	if _, err := fmt.Sscanf(c.Param("tenant_id"), "%d", &tenantID); err != nil || tenantID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tenant_id"})
		return
	}

	var tenant models.Tenant
	if err := s.postgresDB.GetDB().First(&tenant, tenantID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tenant not found"})
		return
	}

	oldPlan := tenant.Plan

	if status, body := s.applyPlanChange(tenantID, req.Plan); status != 0 {
		c.JSON(status, body)
		return
	}

	actor := c.MustGet("super_admin_user").(*models.User)
	_ = s.auditLogSvc.Record(c.Request.Context(), models.AuditLog{
		TenantID:     tenantID,
		ActorUserID:  actor.ID,
		Action:       models.AuditSuperAdminPlanChanged,
		ResourceType: "tenant",
		ResourceID:   fmt.Sprintf("%d", tenantID),
		Category:     models.AuditCategoryAdmin,
		ActorType:    models.AuditActorSuperAdmin,
		UserAgent:    c.Request.UserAgent(),
		Success:      true,
		IPAddress:    c.ClientIP(),
		BeforeJSON:   fmt.Sprintf(`{"plan":"%s"}`, oldPlan),
		AfterJSON:    fmt.Sprintf(`{"plan":"%s"}`, req.Plan),
		Metadata:     "{}",
	})

	s.postgresDB.GetDB().First(&tenant, tenantID)
	c.JSON(http.StatusOK, gin.H{
		"tenant": gin.H{
			"id":     tenant.ID,
			"name":   tenant.Name,
			"plan":   tenant.Plan,
			"status": tenant.Status,
		},
		"plan_limits": models.GetPlanLimits(tenant.Plan),
	})
}

// ── Update tenant status ─────────────────────────────────────────────────────

type updateTenantStatusReq struct {
	Status string `json:"status" binding:"required"`
}

func (s *Server) handleSuperAdminUpdateTenantStatus(c *gin.Context) {
	var req updateTenantStatusReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Status != "active" && req.Status != "suspended" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_status",
			"message": "Status must be \"active\" or \"suspended\".",
		})
		return
	}

	var tenantID uint
	if _, err := fmt.Sscanf(c.Param("tenant_id"), "%d", &tenantID); err != nil || tenantID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tenant_id"})
		return
	}

	db := s.postgresDB.GetDB()
	var tenant models.Tenant
	if err := db.First(&tenant, tenantID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tenant not found"})
		return
	}

	oldStatus := tenant.Status

	if err := db.Model(&tenant).Update("status", req.Status).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update tenant status"})
		return
	}

	actor := c.MustGet("super_admin_user").(*models.User)
	_ = s.auditLogSvc.Record(c.Request.Context(), models.AuditLog{
		TenantID:     tenantID,
		ActorUserID:  actor.ID,
		Action:       models.AuditSuperAdminStatusChanged,
		ResourceType: "tenant",
		ResourceID:   fmt.Sprintf("%d", tenantID),
		Category:     models.AuditCategoryAdmin,
		ActorType:    models.AuditActorSuperAdmin,
		UserAgent:    c.Request.UserAgent(),
		Success:      true,
		IPAddress:    c.ClientIP(),
		BeforeJSON:   fmt.Sprintf(`{"status":"%s"}`, oldStatus),
		AfterJSON:    fmt.Sprintf(`{"status":"%s"}`, req.Status),
		Metadata:     "{}",
	})

	c.JSON(http.StatusOK, gin.H{
		"tenant": gin.H{
			"id":     tenant.ID,
			"name":   tenant.Name,
			"plan":   tenant.Plan,
			"status": req.Status,
		},
	})
}
