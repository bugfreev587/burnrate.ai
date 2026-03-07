package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/xiaoboyu/tokengate/api-server/internal/models"
	"github.com/xiaoboyu/tokengate/api-server/internal/services"
)

// handleListModelGroups returns all model groups for the tenant.
// GET /v1/admin/model-groups
func (s *Server) handleListModelGroups(c *gin.Context) {
	tenantID := c.GetUint("tenant_id")

	groups, err := s.modelGroupSvc.List(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list model groups"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": groups})
}

// handleCreateModelGroup creates a new model group.
// POST /v1/admin/model-groups
func (s *Server) handleCreateModelGroup(c *gin.Context) {
	tenantID := c.GetUint("tenant_id")

	var req struct {
		Name        string                        `json:"name" binding:"required"`
		Strategy    string                        `json:"strategy" binding:"required"`
		Description string                        `json:"description"`
		Enabled     *bool                         `json:"enabled"`
		Deployments []models.ModelGroupDeployment  `json:"deployments" binding:"required,min=1"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate strategy
	if !isValidStrategy(req.Strategy) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid strategy; must be one of: fallback, round-robin, lowest-latency, cost-optimized"})
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	cfg := &models.ModelGroupConfig{
		Name:        req.Name,
		Strategy:    req.Strategy,
		Description: req.Description,
		Enabled:     enabled,
		Deployments: req.Deployments,
	}

	if err := s.modelGroupSvc.Create(c.Request.Context(), tenantID, cfg); err != nil {
		if errors.Is(err, services.ErrModelGroupExists) {
			c.JSON(http.StatusConflict, gin.H{"error": "a model group with this name already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create model group"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": cfg})
}

// handleUpdateModelGroup updates an existing model group.
// PUT /v1/admin/model-groups/:id
func (s *Server) handleUpdateModelGroup(c *gin.Context) {
	tenantID := c.GetUint("tenant_id")
	groupID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid model group ID"})
		return
	}

	var req struct {
		Name        string                        `json:"name" binding:"required"`
		Strategy    string                        `json:"strategy" binding:"required"`
		Description string                        `json:"description"`
		Enabled     bool                          `json:"enabled"`
		Deployments []models.ModelGroupDeployment  `json:"deployments" binding:"required,min=1"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !isValidStrategy(req.Strategy) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid strategy"})
		return
	}

	updates := &models.ModelGroupConfig{
		Name:        req.Name,
		Strategy:    req.Strategy,
		Description: req.Description,
		Enabled:     req.Enabled,
		Deployments: req.Deployments,
	}

	if err := s.modelGroupSvc.Update(c.Request.Context(), tenantID, uint(groupID), updates); err != nil {
		if errors.Is(err, services.ErrModelGroupNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "model group not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update model group"})
		return
	}

	// Reload and return
	cfg, _ := s.modelGroupSvc.GetByID(c.Request.Context(), tenantID, uint(groupID))
	c.JSON(http.StatusOK, gin.H{"data": cfg})
}

// handleDeleteModelGroup deletes a model group.
// DELETE /v1/admin/model-groups/:id
func (s *Server) handleDeleteModelGroup(c *gin.Context) {
	tenantID := c.GetUint("tenant_id")
	groupID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid model group ID"})
		return
	}

	if err := s.modelGroupSvc.Delete(c.Request.Context(), tenantID, uint(groupID)); err != nil {
		if errors.Is(err, services.ErrModelGroupNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "model group not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete model group"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "model group deleted"})
}

// handleGetModelGroupHealth returns health status for all deployments in a model group.
// GET /v1/admin/model-groups/:id/health
func (s *Server) handleGetModelGroupHealth(c *gin.Context) {
	tenantID := c.GetUint("tenant_id")
	groupID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid model group ID"})
		return
	}

	cfg, err := s.modelGroupSvc.GetByID(c.Request.Context(), tenantID, uint(groupID))
	if err != nil {
		if errors.Is(err, services.ErrModelGroupNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "model group not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get model group"})
		return
	}

	// Get health status from the router state
	state := s.smartProxyRouter.State()
	var deploymentHealth []gin.H
	for _, dep := range cfg.Deployments {
		depID := formatDeploymentID(cfg.ID, dep.ID)
		healthy := state.Health.IsHealthy(depID)
		avgLatency := state.Latency.Average(depID)
		deploymentHealth = append(deploymentHealth, gin.H{
			"deployment_id": depID,
			"provider":      dep.Provider,
			"model":         dep.Model,
			"healthy":       healthy,
			"avg_latency_ms": avgLatency.Milliseconds(),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"model_group": cfg.Name,
		"deployments": deploymentHealth,
	})
}

func formatDeploymentID(groupID, depID uint) string {
	return "mg-" + strconv.FormatUint(uint64(groupID), 10) + "-dep-" + strconv.FormatUint(uint64(depID), 10)
}

func isValidStrategy(s string) bool {
	switch s {
	case "fallback", "round-robin", "lowest-latency", "cost-optimized":
		return true
	default:
		return false
	}
}
