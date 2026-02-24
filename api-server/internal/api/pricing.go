package api

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"github.com/xiaoboyu/tokengate/api-server/internal/middleware"
	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

// ─── Providers ─────────────────────────────────────────────────────────────

// GET /v1/admin/pricing/providers
func (s *Server) handleListProviders(c *gin.Context) {
	var providers []models.Provider
	if err := s.postgresDB.GetDB().Find(&providers).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"providers": providers})
}

type createProviderReq struct {
	Name        string `json:"name"         binding:"required"`
	DisplayName string `json:"display_name"`
	Currency    string `json:"currency"`
}

// POST /v1/admin/pricing/providers
func (s *Server) handleCreateProvider(c *gin.Context) {
	var req createProviderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	currency := req.Currency
	if currency == "" {
		currency = "USD"
	}
	provider := &models.Provider{
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Currency:    currency,
	}
	if err := s.postgresDB.GetDB().Create(provider).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, provider)
}

// ─── Models ─────────────────────────────────────────────────────────────────

// GET /v1/admin/pricing/models
func (s *Server) handleListModels(c *gin.Context) {
	var modelDefs []models.ModelDef
	q := s.postgresDB.GetDB()
	if pid := c.Query("provider_id"); pid != "" {
		q = q.Where("provider_id = ?", pid)
	}
	if err := q.Find(&modelDefs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"models": modelDefs})
}

type createModelReq struct {
	ProviderID      uint   `json:"provider_id"       binding:"required"`
	ModelName       string `json:"model_name"        binding:"required"`
	BillingUnitType string `json:"billing_unit_type"`
}

// POST /v1/admin/pricing/models
func (s *Server) handleCreateModel(c *gin.Context) {
	var req createModelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	unitType := req.BillingUnitType
	if unitType == "" {
		unitType = "token"
	}
	model := &models.ModelDef{
		ProviderID:      req.ProviderID,
		ModelName:       req.ModelName,
		BillingUnitType: unitType,
	}
	if err := s.postgresDB.GetDB().Create(model).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, model)
}

// ─── Model Pricing ──────────────────────────────────────────────────────────

// GET /v1/admin/pricing/model-pricing
func (s *Server) handleListModelPricing(c *gin.Context) {
	var pricings []models.ModelPricing
	q := s.postgresDB.GetDB()
	if mid := c.Query("model_id"); mid != "" {
		q = q.Where("model_id = ?", mid)
	}
	if err := q.Order("effective_from DESC").Find(&pricings).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"model_pricing": pricings})
}

type createModelPricingReq struct {
	ModelID       uint   `json:"model_id"        binding:"required"`
	PriceType     string `json:"price_type"      binding:"required"`
	PricePerUnit  string `json:"price_per_unit"  binding:"required"` // decimal string
	UnitSize      int64  `json:"unit_size"`
	EffectiveFrom string `json:"effective_from"  binding:"required"` // RFC3339
	EffectiveTo   string `json:"effective_to"`
}

// POST /v1/admin/pricing/model-pricing
func (s *Server) handleCreateModelPricing(c *gin.Context) {
	var req createModelPricingReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	price, err := decimal.NewFromString(req.PricePerUnit)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid price_per_unit: " + err.Error()})
		return
	}
	from, err := time.Parse(time.RFC3339, req.EffectiveFrom)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid effective_from: " + err.Error()})
		return
	}
	unitSize := req.UnitSize
	if unitSize == 0 {
		unitSize = 1_000_000
	}
	mp := &models.ModelPricing{
		ModelID:       req.ModelID,
		PriceType:     req.PriceType,
		PricePerUnit:  price,
		UnitSize:      unitSize,
		EffectiveFrom: from,
	}
	if req.EffectiveTo != "" {
		to, err := time.Parse(time.RFC3339, req.EffectiveTo)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid effective_to: " + err.Error()})
			return
		}
		mp.EffectiveTo = &to
	}
	if err := s.postgresDB.GetDB().Create(mp).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, mp)
}

// ─── Markups ────────────────────────────────────────────────────────────────

// GET /v1/admin/pricing/markups
func (s *Server) handleListMarkups(c *gin.Context) {
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var markups []models.PricingMarkup
	if err := s.postgresDB.GetDB().Where("tenant_id = ?", tenantID).Find(&markups).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"markups": markups})
}

type createMarkupReq struct {
	ProviderID    *uint  `json:"provider_id"`
	ModelID       *uint  `json:"model_id"`
	Percentage    string `json:"percentage"     binding:"required"` // decimal string
	Priority      int    `json:"priority"`
	EffectiveFrom string `json:"effective_from" binding:"required"`
	EffectiveTo   string `json:"effective_to"`
}

// POST /v1/admin/pricing/markups
func (s *Server) handleCreateMarkup(c *gin.Context) {
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var req createMarkupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	pct, err := decimal.NewFromString(req.Percentage)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid percentage: " + err.Error()})
		return
	}
	from, err := time.Parse(time.RFC3339, req.EffectiveFrom)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid effective_from: " + err.Error()})
		return
	}
	markup := &models.PricingMarkup{
		TenantID:      tenantID,
		ProviderID:    req.ProviderID,
		ModelID:       req.ModelID,
		Percentage:    pct,
		Priority:      req.Priority,
		EffectiveFrom: from,
	}
	if req.EffectiveTo != "" {
		to, err := time.Parse(time.RFC3339, req.EffectiveTo)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid effective_to: " + err.Error()})
			return
		}
		markup.EffectiveTo = &to
	}
	if err := s.postgresDB.GetDB().Create(markup).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, markup)
}

// DELETE /v1/admin/pricing/markups/:markup_id
func (s *Server) handleDeleteMarkup(c *gin.Context) {
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	id, err := strconv.ParseUint(c.Param("markup_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid markup_id"})
		return
	}
	result := s.postgresDB.GetDB().
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Delete(&models.PricingMarkup{})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "markup not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// ─── Contracts ──────────────────────────────────────────────────────────────

// GET /v1/admin/pricing/contracts
func (s *Server) handleListContracts(c *gin.Context) {
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var contracts []models.ContractPricing
	if err := s.postgresDB.GetDB().Where("tenant_id = ?", tenantID).Find(&contracts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"contracts": contracts})
}

type createContractReq struct {
	ModelID       uint   `json:"model_id"        binding:"required"`
	PriceType     string `json:"price_type"      binding:"required"`
	PriceOverride string `json:"price_override"  binding:"required"` // decimal string
	UnitSize      int64  `json:"unit_size"`
	EffectiveFrom string `json:"effective_from"  binding:"required"`
	EffectiveTo   string `json:"effective_to"`
}

// POST /v1/admin/pricing/contracts
func (s *Server) handleCreateContract(c *gin.Context) {
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var req createContractReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	price, err := decimal.NewFromString(req.PriceOverride)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid price_override: " + err.Error()})
		return
	}
	from, err := time.Parse(time.RFC3339, req.EffectiveFrom)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid effective_from: " + err.Error()})
		return
	}
	unitSize := req.UnitSize
	if unitSize == 0 {
		unitSize = 1_000_000
	}
	contract := &models.ContractPricing{
		TenantID:      tenantID,
		ModelID:       req.ModelID,
		PriceType:     req.PriceType,
		PriceOverride: price,
		UnitSize:      unitSize,
		EffectiveFrom: from,
	}
	if req.EffectiveTo != "" {
		to, err := time.Parse(time.RFC3339, req.EffectiveTo)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid effective_to: " + err.Error()})
			return
		}
		contract.EffectiveTo = &to
	}
	if err := s.postgresDB.GetDB().Create(contract).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, contract)
}

// ─── Budget ─────────────────────────────────────────────────────────────────

// GET /v1/admin/budget
func (s *Server) handleGetBudget(c *gin.Context) {
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var limits []models.BudgetLimit
	if err := s.postgresDB.GetDB().Where("tenant_id = ?", tenantID).Find(&limits).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Compute current period spend for each limit.
	loc := parseTimezone(c)
	now := time.Now().In(loc)
	out := make([]gin.H, len(limits))
	for i, bl := range limits {
		var periodStart time.Time
		switch bl.PeriodType {
		case models.PeriodWeekly:
			weekday := int(now.Weekday())
			if weekday == 0 {
				weekday = 7 // Sunday → 7 so Monday = start
			}
			periodStart = time.Date(now.Year(), now.Month(), now.Day()-weekday+1, 0, 0, 0, 0, loc)
		case models.PeriodDaily:
			periodStart = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
		default: // monthly
			periodStart = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
		}

		q := s.postgresDB.GetDB().Model(&models.UsageLog{}).
			Where("tenant_id = ? AND created_at >= ? AND api_usage_billed = ?", tenantID, periodStart, true)
		if bl.ScopeType == models.BudgetScopeAPIKey && bl.ScopeID != "" {
			q = q.Where("request_id IN (SELECT request_id FROM usage_logs WHERE tenant_id = ?)", tenantID)
			// scope to key: UsageLog doesn't store key_id, so account-level spend is the best proxy
		}
		var currentSpend decimal.Decimal
		q.Select("COALESCE(SUM(cost), 0)").Scan(&currentSpend)

		pct := 0.0
		if bl.LimitAmount.IsPositive() {
			pct, _ = currentSpend.Div(bl.LimitAmount).Mul(decimal.NewFromInt(100)).Float64()
			if pct > 100 {
				pct = 100
			}
		}

		out[i] = gin.H{
			"id":              bl.ID,
			"scope_type":      bl.ScopeType,
			"scope_id":        bl.ScopeID,
			"period_type":     bl.PeriodType,
			"limit_amount":    bl.LimitAmount.StringFixed(2),
			"alert_threshold": bl.AlertThreshold.StringFixed(0),
			"action":          bl.Action,
			"current_spend":   currentSpend.StringFixed(4),
			"pct_used":        math.Round(pct*10) / 10,
			"period_start":    periodStart.Format("2006-01-02"),
			"created_at":      bl.CreatedAt,
		}
	}

	c.JSON(http.StatusOK, gin.H{"budget_limits": out})
}

type upsertBudgetReq struct {
	ScopeType      string `json:"scope_type"`                         // account|api_key (default: account)
	ScopeID        string `json:"scope_id"`                           // "" for account, key_id for api_key
	PeriodType     string `json:"period_type"     binding:"required"` // monthly|weekly|daily
	LimitAmount    string `json:"limit_amount"    binding:"required"` // decimal string
	AlertThreshold string `json:"alert_threshold"`                    // percentage, default 80
	Action         string `json:"action"`                             // alert|block
}

// PUT /v1/admin/budget
func (s *Server) handleUpsertBudget(c *gin.Context) {
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var req upsertBudgetReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	scopeType := req.ScopeType
	if scopeType == "" {
		scopeType = models.BudgetScopeAccount
	}
	if scopeType != models.BudgetScopeAccount && scopeType != models.BudgetScopeAPIKey {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scope_type must be 'account' or 'api_key'"})
		return
	}
	limit, err := decimal.NewFromString(req.LimitAmount)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit_amount: " + err.Error()})
		return
	}
	action := req.Action
	if action == "" {
		action = models.BudgetActionAlert
	}
	alertThreshold := decimal.NewFromInt(80)
	if req.AlertThreshold != "" {
		if t, err := decimal.NewFromString(req.AlertThreshold); err == nil {
			alertThreshold = t
		}
	}

	// Enforce plan-based budget restrictions.
	var tenant models.Tenant
	if err := s.postgresDB.GetDB().First(&tenant, tenantID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch tenant"})
		return
	}
	planLim := models.GetPlanLimits(tenant.Plan)

	periodAllowed := false
	for _, p := range planLim.AllowedPeriods {
		if p == req.PeriodType {
			periodAllowed = true
			break
		}
	}
	if !periodAllowed {
		c.JSON(http.StatusForbidden, gin.H{
			"error":           "plan_restriction",
			"message":         fmt.Sprintf("Your %s plan only supports these budget periods: %v. Upgrade for weekly/daily limits.", tenant.Plan, planLim.AllowedPeriods),
			"allowed_periods": planLim.AllowedPeriods,
			"plan":            tenant.Plan,
		})
		return
	}
	if (action == models.BudgetActionBlock || action == models.BudgetActionAlertBlock) && !planLim.AllowBlockAction {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "plan_restriction",
			"message": fmt.Sprintf("Your %s plan does not support automatic blocking. Upgrade to Pro or higher.", tenant.Plan),
			"plan":    tenant.Plan,
		})
		return
	}
	if scopeType == models.BudgetScopeAPIKey && !planLim.AllowPerKeyBudget {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "plan_restriction",
			"message": fmt.Sprintf("Your %s plan does not support per-API-key budget limits. Upgrade to Team or higher.", tenant.Plan),
			"plan":    tenant.Plan,
		})
		return
	}

	budgetLimit := models.BudgetLimit{
		TenantID:       tenantID,
		ScopeType:      scopeType,
		ScopeID:        req.ScopeID,
		PeriodType:     req.PeriodType,
		LimitAmount:    limit,
		AlertThreshold: alertThreshold,
		Action:         action,
	}

	// Upsert: find by (tenant_id, scope_type, scope_id, period_type) and update, or create.
	result := s.postgresDB.GetDB().
		Where("tenant_id = ? AND scope_type = ? AND scope_id = ? AND period_type = ?",
			tenantID, scopeType, req.ScopeID, req.PeriodType).
		Assign(models.BudgetLimit{
			LimitAmount:    limit,
			AlertThreshold: alertThreshold,
			Action:         action,
		}).
		FirstOrCreate(&budgetLimit)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
		return
	}
	if result.RowsAffected == 0 {
		s.postgresDB.GetDB().Model(&budgetLimit).Updates(map[string]interface{}{
			"limit_amount":    limit,
			"alert_threshold": alertThreshold,
			"action":          action,
		})
	}

	c.JSON(http.StatusOK, budgetLimit)
}

// DELETE /v1/admin/budget/:budget_id
func (s *Server) handleDeleteBudget(c *gin.Context) {
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	id, err := strconv.ParseUint(c.Param("budget_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid budget_id"})
		return
	}
	result := s.postgresDB.GetDB().
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Delete(&models.BudgetLimit{})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "budget limit not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// ─── Cost Ledger ─────────────────────────────────────────────────────────────

// GET /v1/cost-ledger
func (s *Server) handleListCostLedger(c *gin.Context) {
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}
	offset := (page - 1) * limit

	// Apply plan-based data retention as the floor for the time window.
	var tenant models.Tenant
	s.postgresDB.GetDB().First(&tenant, tenantID)
	planLim := models.GetPlanLimits(tenant.Plan)

	q := s.postgresDB.GetDB().Where("tenant_id = ?", tenantID)
	if planLim.DataRetentionDays > 0 {
		retentionCutoff := time.Now().AddDate(0, 0, -planLim.DataRetentionDays)
		q = q.Where("timestamp >= ?", retentionCutoff)
	}
	if from := c.Query("from"); from != "" {
		if t, err := time.Parse(time.RFC3339, from); err == nil {
			q = q.Where("timestamp >= ?", t)
		}
	}
	if to := c.Query("to"); to != "" {
		if t, err := time.Parse(time.RFC3339, to); err == nil {
			q = q.Where("timestamp <= ?", t)
		}
	}

	var total int64
	q.Model(&models.CostLedger{}).Count(&total)

	var entries []models.CostLedger
	if err := q.Order("timestamp DESC").Offset(offset).Limit(limit).Find(&entries).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"entries": entries,
		"total":   total,
		"page":    page,
		"limit":   limit,
	})
}

// ─── Usage Forecast ──────────────────────────────────────────────────────────

type forecastResult struct {
	TotalSoFar    string `json:"total_so_far"`
	DailyAverage  string `json:"daily_average"`
	Forecast      string `json:"forecast"`
	DaysElapsed   int    `json:"days_elapsed"`
	DaysRemaining int    `json:"days_remaining"`
	Month         string `json:"month"`
}

// GET /v1/usage/forecast
func (s *Server) handleUsageForecast(c *gin.Context) {
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	loc := parseTimezone(c)
	now := time.Now().In(loc)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	monthEnd := monthStart.AddDate(0, 1, 0)
	daysInMonth := int(monthEnd.Sub(monthStart).Hours() / 24)

	// SUM(final_cost) and COUNT(DISTINCT DATE(timestamp)) for current month
	type result struct {
		TotalCost  decimal.Decimal
		DaysWithData int
	}

	var totalCost decimal.Decimal
	s.postgresDB.GetDB().
		Model(&models.CostLedger{}).
		Select("COALESCE(SUM(final_cost), 0)").
		Where("tenant_id = ? AND timestamp >= ? AND timestamp < ? AND api_usage_billed = ?", tenantID, monthStart, monthEnd, true).
		Scan(&totalCost)

	var daysWithData int64
	tzName := loc.String()
	s.postgresDB.GetDB().
		Model(&models.CostLedger{}).
		Select(fmt.Sprintf("COUNT(DISTINCT DATE(timestamp AT TIME ZONE '%s'))", tzName)).
		Where("tenant_id = ? AND timestamp >= ? AND timestamp < ? AND api_usage_billed = ?", tenantID, monthStart, monthEnd, true).
		Scan(&daysWithData)

	daysElapsed := int(daysWithData)
	if daysElapsed < 3 {
		daysElapsed = 3
	}

	daysRemaining := daysInMonth - now.Day()

	dailyAvg := totalCost.Div(decimal.NewFromInt(int64(daysElapsed)))
	forecast := totalCost.Add(dailyAvg.Mul(decimal.NewFromInt(int64(daysRemaining))))

	c.JSON(http.StatusOK, forecastResult{
		TotalSoFar:    totalCost.StringFixed(8),
		DailyAverage:  dailyAvg.StringFixed(8),
		Forecast:      forecast.StringFixed(8),
		DaysElapsed:   daysElapsed,
		DaysRemaining: daysRemaining,
		Month:         monthStart.Format("2006-01"),
	})
}

// ─── Pricing Catalog ──────────────────────────────────────────────────────────

type catalogPriceEntry struct {
	PricePerUnit string `json:"price_per_unit"`
	UnitSize     int64  `json:"unit_size"`
	PricingID    uint   `json:"pricing_id"`
}

type catalogEntry struct {
	ProviderID      uint                         `json:"provider_id"`
	Provider        string                       `json:"provider"`
	ProviderDisplay string                       `json:"provider_display"`
	ModelID         uint                         `json:"model_id"`
	ModelName       string                       `json:"model_name"`
	Prices          map[string]catalogPriceEntry `json:"prices"`
}

// GET /v1/admin/pricing/catalog
// Returns all models with their current effective pricing, pre-joined for the UI.
func (s *Server) handleGetPricingCatalog(c *gin.Context) {
	db := s.postgresDB.GetDB()
	now := time.Now()

	var providers []models.Provider
	db.Find(&providers)
	providerByID := make(map[uint]models.Provider, len(providers))
	for _, p := range providers {
		providerByID[p.ID] = p
	}

	var modelDefs []models.ModelDef
	db.Find(&modelDefs)

	var pricings []models.ModelPricing
	db.Where("effective_from <= ? AND (effective_to IS NULL OR effective_to > ?)", now, now).Find(&pricings)

	// Group prices by model_id
	pricesByModel := make(map[uint][]models.ModelPricing)
	for _, mp := range pricings {
		pricesByModel[mp.ModelID] = append(pricesByModel[mp.ModelID], mp)
	}

	catalog := make([]catalogEntry, 0, len(modelDefs))
	for _, m := range modelDefs {
		prov := providerByID[m.ProviderID]
		entry := catalogEntry{
			ProviderID:      prov.ID,
			Provider:        prov.Name,
			ProviderDisplay: prov.DisplayName,
			ModelID:         m.ID,
			ModelName:       m.ModelName,
			Prices:          make(map[string]catalogPriceEntry),
		}
		for _, mp := range pricesByModel[m.ID] {
			entry.Prices[mp.PriceType] = catalogPriceEntry{
				PricePerUnit: mp.PricePerUnit.String(),
				UnitSize:     mp.UnitSize,
				PricingID:    mp.ID,
			}
		}
		catalog = append(catalog, entry)
	}

	c.JSON(http.StatusOK, gin.H{"catalog": catalog})
}

// ─── Pricing Configs ─────────────────────────────────────────────────────────

type configRateView struct {
	ID           uint   `json:"id"`
	ModelID      uint   `json:"model_id"`
	ModelName    string `json:"model_name"`
	Provider     string `json:"provider"`
	PriceType    string `json:"price_type"`
	PricePerUnit string `json:"price_per_unit"`
	UnitSize     int64  `json:"unit_size"`
}

type pricingConfigView struct {
	ID          uint             `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Rates       []configRateView `json:"rates"`
	AssignedKey *assignedKeyView `json:"assigned_key"`
	CreatedAt   time.Time        `json:"created_at"`
}

type assignedKeyView struct {
	KeyID string `json:"key_id"`
	Label string `json:"label"`
}

// GET /v1/admin/pricing-configs
func (s *Server) handleListPricingConfigs(c *gin.Context) {
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	db := s.postgresDB.GetDB()

	var configs []models.PricingConfig
	db.Where("tenant_id = ?", tenantID).Order("created_at DESC").Find(&configs)

	// Preload all rates + model info in bulk
	configIDs := make([]uint, len(configs))
	for i, cfg := range configs {
		configIDs[i] = cfg.ID
	}
	var rates []models.PricingConfigRate
	db.Where("config_id IN ?", configIDs).Find(&rates)

	// Preload model+provider info
	modelIDs := make([]uint, 0, len(rates))
	seen := map[uint]bool{}
	for _, r := range rates {
		if !seen[r.ModelID] {
			modelIDs = append(modelIDs, r.ModelID)
			seen[r.ModelID] = true
		}
	}
	var modelDefs []models.ModelDef
	db.Where("id IN ?", modelIDs).Find(&modelDefs)
	var providers []models.Provider
	db.Find(&providers)

	modelByID := make(map[uint]models.ModelDef)
	for _, m := range modelDefs {
		modelByID[m.ID] = m
	}
	provByID := make(map[uint]models.Provider)
	for _, p := range providers {
		provByID[p.ID] = p
	}

	// Preload assignments
	var assignments []models.APIKeyConfig
	db.Where("config_id IN ?", configIDs).Find(&assignments)

	// Preload API keys for assignments
	keyIDs := make([]string, 0, len(assignments))
	for _, a := range assignments {
		keyIDs = append(keyIDs, a.KeyID)
	}
	var apiKeys []models.APIKey
	if len(keyIDs) > 0 {
		db.Where("key_id IN ?", keyIDs).Find(&apiKeys)
	}
	keyByID := make(map[string]models.APIKey)
	for _, k := range apiKeys {
		keyByID[k.KeyID] = k
	}
	assignByConfig := make(map[uint]models.APIKeyConfig)
	for _, a := range assignments {
		assignByConfig[a.ConfigID] = a
	}
	ratesByConfig := make(map[uint][]models.PricingConfigRate)
	for _, r := range rates {
		ratesByConfig[r.ConfigID] = append(ratesByConfig[r.ConfigID], r)
	}

	out := make([]pricingConfigView, 0, len(configs))
	for _, cfg := range configs {
		view := pricingConfigView{
			ID:          cfg.ID,
			Name:        cfg.Name,
			Description: cfg.Description,
			CreatedAt:   cfg.CreatedAt,
			Rates:       []configRateView{},
		}
		for _, r := range ratesByConfig[cfg.ID] {
			m := modelByID[r.ModelID]
			p := provByID[m.ProviderID]
			view.Rates = append(view.Rates, configRateView{
				ID:           r.ID,
				ModelID:      r.ModelID,
				ModelName:    m.ModelName,
				Provider:     p.Name,
				PriceType:    r.PriceType,
				PricePerUnit: r.PricePerUnit.String(),
				UnitSize:     r.UnitSize,
			})
		}
		if a, hasAssign := assignByConfig[cfg.ID]; hasAssign {
			if k, hasKey := keyByID[a.KeyID]; hasKey {
				view.AssignedKey = &assignedKeyView{KeyID: k.KeyID, Label: k.Label}
			}
		}
		out = append(out, view)
	}

	c.JSON(http.StatusOK, gin.H{"configs": out})
}

type createPricingConfigReq struct {
	Name        string `json:"name"        binding:"required"`
	Description string `json:"description"`
}

// POST /v1/admin/pricing-configs
func (s *Server) handleCreatePricingConfig(c *gin.Context) {
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var req createPricingConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cfg := &models.PricingConfig{TenantID: tenantID, Name: req.Name, Description: req.Description}
	if err := s.postgresDB.GetDB().Create(cfg).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"id": cfg.ID, "name": cfg.Name, "description": cfg.Description,
		"rates": []configRateView{}, "assigned_key": nil, "created_at": cfg.CreatedAt,
	})
}

// DELETE /v1/admin/pricing-configs/:config_id
func (s *Server) handleDeletePricingConfig(c *gin.Context) {
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	id, err := strconv.ParseUint(c.Param("config_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid config_id"})
		return
	}
	db := s.postgresDB.GetDB()
	// Verify ownership
	var cfg models.PricingConfig
	if err := db.Where("id = ? AND tenant_id = ?", id, tenantID).First(&cfg).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "config not found"})
		return
	}
	db.Where("config_id = ?", id).Delete(&models.PricingConfigRate{})
	db.Where("config_id = ?", id).Delete(&models.APIKeyConfig{})
	db.Delete(&cfg)
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// ─── Pricing Config Rates ────────────────────────────────────────────────────

type addConfigRateReq struct {
	ModelID      uint   `json:"model_id"      binding:"required"`
	PriceType    string `json:"price_type"    binding:"required"`
	PricePerUnit string `json:"price_per_unit" binding:"required"`
	UnitSize     int64  `json:"unit_size"`
}

// POST /v1/admin/pricing-configs/:config_id/rates
func (s *Server) handleAddConfigRate(c *gin.Context) {
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	configID, err := strconv.ParseUint(c.Param("config_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid config_id"})
		return
	}
	var req addConfigRateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	price, err := decimal.NewFromString(req.PricePerUnit)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid price_per_unit"})
		return
	}
	db := s.postgresDB.GetDB()
	var cfg models.PricingConfig
	if err := db.Where("id = ? AND tenant_id = ?", configID, tenantID).First(&cfg).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "config not found"})
		return
	}
	unitSize := req.UnitSize
	if unitSize == 0 {
		unitSize = 1_000_000
	}
	// Upsert: one row per (config_id, model_id, price_type)
	var existing models.PricingConfigRate
	result := db.Where("config_id = ? AND model_id = ? AND price_type = ?", configID, req.ModelID, req.PriceType).First(&existing)
	if result.Error == nil {
		db.Model(&existing).Updates(map[string]interface{}{
			"price_per_unit": price,
			"unit_size":      unitSize,
		})
		existing.PricePerUnit = price
		existing.UnitSize = unitSize

		var m models.ModelDef
		db.First(&m, existing.ModelID)
		var prov models.Provider
		db.First(&prov, m.ProviderID)
		c.JSON(http.StatusOK, configRateView{
			ID: existing.ID, ModelID: existing.ModelID, ModelName: m.ModelName,
			Provider: prov.Name, PriceType: existing.PriceType,
			PricePerUnit: existing.PricePerUnit.String(), UnitSize: existing.UnitSize,
		})
		return
	}
	rate := &models.PricingConfigRate{
		ConfigID: uint(configID), ModelID: req.ModelID,
		PriceType: req.PriceType, PricePerUnit: price, UnitSize: unitSize,
	}
	if err := db.Create(rate).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var m models.ModelDef
	db.First(&m, rate.ModelID)
	var prov models.Provider
	db.First(&prov, m.ProviderID)
	c.JSON(http.StatusCreated, configRateView{
		ID: rate.ID, ModelID: rate.ModelID, ModelName: m.ModelName,
		Provider: prov.Name, PriceType: rate.PriceType,
		PricePerUnit: rate.PricePerUnit.String(), UnitSize: rate.UnitSize,
	})
}

// DELETE /v1/admin/pricing-configs/:config_id/rates/:rate_id
func (s *Server) handleDeleteConfigRate(c *gin.Context) {
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	configID, err := strconv.ParseUint(c.Param("config_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid config_id"})
		return
	}
	rateID, err := strconv.ParseUint(c.Param("rate_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rate_id"})
		return
	}
	db := s.postgresDB.GetDB()
	var cfg models.PricingConfig
	if err := db.Where("id = ? AND tenant_id = ?", configID, tenantID).First(&cfg).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "config not found"})
		return
	}
	res := db.Where("id = ? AND config_id = ?", rateID, configID).Delete(&models.PricingConfigRate{})
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "rate not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// ─── Pricing Config Assignment ────────────────────────────────────────────────

type assignConfigReq struct {
	KeyID string `json:"key_id" binding:"required"`
}

// PUT /v1/admin/pricing-configs/:config_id/assign
func (s *Server) handleAssignPricingConfig(c *gin.Context) {
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	configID, err := strconv.ParseUint(c.Param("config_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid config_id"})
		return
	}
	var req assignConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	db := s.postgresDB.GetDB()
	// Verify config belongs to tenant
	var cfg models.PricingConfig
	if err := db.Where("id = ? AND tenant_id = ?", configID, tenantID).First(&cfg).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "config not found"})
		return
	}
	// Verify API key belongs to tenant and is active
	var apiKey models.APIKey
	if err := db.Where("key_id = ? AND tenant_id = ? AND revoked = false", req.KeyID, tenantID).First(&apiKey).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "active API key not found"})
		return
	}
	// Upsert: remove any existing assignment for this key, then create
	db.Where("key_id = ?", req.KeyID).Delete(&models.APIKeyConfig{})
	assignment := &models.APIKeyConfig{
		TenantID: tenantID,
		KeyID:    req.KeyID,
		ConfigID: uint(configID),
	}
	if err := db.Create(assignment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"config_id": configID,
		"key_id":    req.KeyID,
		"label":     apiKey.Label,
	})
}

// DELETE /v1/admin/pricing-configs/:config_id/assign
func (s *Server) handleUnassignPricingConfig(c *gin.Context) {
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	configID, err := strconv.ParseUint(c.Param("config_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid config_id"})
		return
	}
	db := s.postgresDB.GetDB()
	var cfg models.PricingConfig
	if err := db.Where("id = ? AND tenant_id = ?", configID, tenantID).First(&cfg).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "config not found"})
		return
	}
	db.Where("config_id = ?", configID).Delete(&models.APIKeyConfig{})
	c.JSON(http.StatusOK, gin.H{"unassigned": true})
}
