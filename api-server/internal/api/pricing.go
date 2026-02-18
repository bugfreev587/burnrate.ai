package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"github.com/xiaoboyu/burnrate-ai/api-server/internal/middleware"
	"github.com/xiaoboyu/burnrate-ai/api-server/internal/models"
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
	c.JSON(http.StatusOK, gin.H{"budget_limits": limits})
}

type upsertBudgetReq struct {
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

	budgetLimit := models.BudgetLimit{
		TenantID:       tenantID,
		PeriodType:     req.PeriodType,
		LimitAmount:    limit,
		AlertThreshold: alertThreshold,
		Action:         action,
	}

	// Upsert: update if exists, create if not
	result := s.postgresDB.GetDB().
		Where("tenant_id = ? AND period_type = ?", tenantID, req.PeriodType).
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
	// If it already existed, update the fields
	if result.RowsAffected == 0 {
		s.postgresDB.GetDB().Model(&budgetLimit).Updates(map[string]interface{}{
			"limit_amount":    limit,
			"alert_threshold": alertThreshold,
			"action":          action,
		})
	}

	c.JSON(http.StatusOK, budgetLimit)
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

	q := s.postgresDB.GetDB().Where("tenant_id = ?", tenantID)
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

	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
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
		Where("tenant_id = ? AND timestamp >= ? AND timestamp < ?", tenantID, monthStart, monthEnd).
		Scan(&totalCost)

	var daysWithData int64
	s.postgresDB.GetDB().
		Model(&models.CostLedger{}).
		Select("COUNT(DISTINCT DATE(timestamp))").
		Where("tenant_id = ? AND timestamp >= ? AND timestamp < ?", tenantID, monthStart, monthEnd).
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
