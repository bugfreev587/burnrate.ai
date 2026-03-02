package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/xiaoboyu/tokengate/api-server/internal/middleware"
	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

// ── Project CRUD ────────────────────────────────────────────────────────────

type projectResponse struct {
	ID          uint      `json:"id"`
	TenantID    uint      `json:"tenant_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	IsDefault   bool      `json:"is_default"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func toProjectResponse(p models.Project, defaultProjectID *uint) projectResponse {
	isDefault := defaultProjectID != nil && *defaultProjectID == p.ID
	return projectResponse{
		ID:          p.ID,
		TenantID:    p.TenantID,
		Name:        p.Name,
		Description: p.Description,
		Status:      p.Status,
		IsDefault:   isDefault,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}

// handleListProjects lists projects in the tenant.
// Owner/Admin see all; Editor/Viewer see only their project memberships.
// GET /v1/projects
func (s *Server) handleListProjects(c *gin.Context) {
	user, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	tenantID, _ := middleware.GetTenantIDFromContext(c)
	orgRole := middleware.GetOrgRoleFromContext(c)
	db := s.postgresDB.GetDB()

	var projects []models.Project
	if orgRole == models.RoleOwner || orgRole == models.RoleAdmin {
		db.Where("tenant_id = ? AND status = ?", tenantID, models.ProjectStatusActive).
			Order("created_at ASC").Find(&projects)
	} else {
		// Editor/Viewer: only projects they are members of.
		db.Table("projects").
			Joins("JOIN project_memberships ON project_memberships.project_id = projects.id").
			Where("projects.tenant_id = ? AND projects.status = ? AND project_memberships.user_id = ?",
				tenantID, models.ProjectStatusActive, user.ID).
			Order("projects.created_at ASC").Find(&projects)
	}

	var tenant models.Tenant
	db.First(&tenant, tenantID)
	planLim := models.GetPlanLimits(tenant.Plan)

	out := make([]projectResponse, len(projects))
	for i, p := range projects {
		out[i] = toProjectResponse(p, tenant.DefaultProjectID)
	}

	var limitResp, slotsLeft interface{}
	if planLim.MaxProjects != -1 {
		limitResp = planLim.MaxProjects
		slotsLeft = planLim.MaxProjects - len(out)
	}

	c.JSON(http.StatusOK, gin.H{
		"projects":   out,
		"count":      len(out),
		"limit":      limitResp,
		"slots_left": slotsLeft,
		"plan":       tenant.Plan,
	})
}

type createProjectReq struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

// handleCreateProject creates a new project in the tenant.
// POST /v1/projects
func (s *Server) handleCreateProject(c *gin.Context) {
	user, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	tenantID, _ := middleware.GetTenantIDFromContext(c)

	var req createProjectReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	db := s.postgresDB.GetDB()

	// Enforce plan limit.
	var tenant models.Tenant
	db.First(&tenant, tenantID)
	planLim := models.GetPlanLimits(tenant.Plan)
	if planLim.MaxProjects != -1 {
		var count int64
		db.Model(&models.Project{}).Where("tenant_id = ? AND status = ?", tenantID, models.ProjectStatusActive).Count(&count)
		if int(count) >= planLim.MaxProjects {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error":   "plan_limit_reached",
				"message": fmt.Sprintf("Your %s plan allows up to %d project(s). Upgrade to add more.", tenant.Plan, planLim.MaxProjects),
				"limit":   planLim.MaxProjects,
				"current": count,
				"plan":    tenant.Plan,
			})
			return
		}
	}

	// Check uniqueness.
	var existing models.Project
	if err := db.Where("tenant_id = ? AND name = ?", tenantID, req.Name).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "a project with this name already exists"})
		return
	}

	project := models.Project{
		TenantID:    tenantID,
		Name:        req.Name,
		Description: req.Description,
		Status:      models.ProjectStatusActive,
	}
	if err := db.Create(&project).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create project"})
		return
	}

	// Auto-add creator as project_admin.
	membership := models.ProjectMembership{
		ProjectID:   project.ID,
		UserID:      user.ID,
		ProjectRole: models.ProjectRoleAdmin,
	}
	db.Create(&membership)

	s.recordAudit(c, "project:create", "project", fmt.Sprintf("%d", project.ID))

	c.JSON(http.StatusCreated, toProjectResponse(project, tenant.DefaultProjectID))
}

// handleGetProject returns a single project.
// GET /v1/projects/:id
func (s *Server) handleGetProject(c *gin.Context) {
	_, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	tenantID, _ := middleware.GetTenantIDFromContext(c)

	projectID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}

	db := s.postgresDB.GetDB()
	var project models.Project
	if err := db.Where("id = ? AND tenant_id = ?", projectID, tenantID).First(&project).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	var tenant models.Tenant
	db.First(&tenant, tenantID)
	c.JSON(http.StatusOK, toProjectResponse(project, tenant.DefaultProjectID))
}

type updateProjectReq struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

// handleUpdateProject updates a project's name or description.
// PATCH /v1/projects/:id
func (s *Server) handleUpdateProject(c *gin.Context) {
	_, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	tenantID, _ := middleware.GetTenantIDFromContext(c)

	projectID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}

	var req updateProjectReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	db := s.postgresDB.GetDB()
	var project models.Project
	if err := db.Where("id = ? AND tenant_id = ?", projectID, tenantID).First(&project).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	updates := map[string]interface{}{}
	if req.Name != nil {
		// Check uniqueness.
		var existing models.Project
		if err := db.Where("tenant_id = ? AND name = ? AND id != ?", tenantID, *req.Name, projectID).First(&existing).Error; err == nil {
			c.JSON(http.StatusConflict, gin.H{"error": "a project with this name already exists"})
			return
		}
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no fields to update"})
		return
	}

	db.Model(&project).Updates(updates)

	s.recordAudit(c, "project:update", "project", fmt.Sprintf("%d", projectID))

	var tenant models.Tenant
	db.First(&tenant, tenantID)
	db.First(&project, projectID)
	c.JSON(http.StatusOK, toProjectResponse(project, tenant.DefaultProjectID))
}

// handleDeleteProject deletes (archives) a project.
// DELETE /v1/projects/:id
func (s *Server) handleDeleteProject(c *gin.Context) {
	_, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	tenantID, _ := middleware.GetTenantIDFromContext(c)

	projectID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}

	db := s.postgresDB.GetDB()
	var project models.Project
	if err := db.Where("id = ? AND tenant_id = ?", projectID, tenantID).First(&project).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	// Block deletion of the default project.
	var tenant models.Tenant
	db.First(&tenant, tenantID)
	if tenant.DefaultProjectID != nil && *tenant.DefaultProjectID == uint(projectID) {
		c.JSON(http.StatusConflict, gin.H{
			"error":   "cannot_delete_default",
			"message": "The default project cannot be deleted.",
		})
		return
	}

	// Block deletion if API keys reference this project.
	var keyCount int64
	db.Model(&models.APIKey{}).Where("project_id = ? AND revoked = false", projectID).Count(&keyCount)
	if keyCount > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"error":   "has_active_keys",
			"message": fmt.Sprintf("Cannot delete project: %d active API key(s) reference it. Revoke them first.", keyCount),
		})
		return
	}

	// Archive the project.
	db.Model(&project).Update("status", models.ProjectStatusArchived)
	// Clean up project memberships.
	db.Where("project_id = ?", projectID).Delete(&models.ProjectMembership{})

	s.recordAudit(c, "project:delete", "project", fmt.Sprintf("%d", projectID))

	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// ── Project Members ─────────────────────────────────────────────────────────

type projectMemberResponse struct {
	UserID      string    `json:"user_id"`
	Email       string    `json:"email"`
	Name        string    `json:"name"`
	ProjectRole string    `json:"project_role"`
	CreatedAt   time.Time `json:"created_at"`
}

// handleListProjectMembers lists members of a project.
// GET /v1/projects/:id/members
func (s *Server) handleListProjectMembers(c *gin.Context) {
	_, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	tenantID, _ := middleware.GetTenantIDFromContext(c)

	projectID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}

	db := s.postgresDB.GetDB()

	// Validate project belongs to tenant.
	var project models.Project
	if err := db.Where("id = ? AND tenant_id = ?", projectID, tenantID).First(&project).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	type memberRow struct {
		UserID      string    `gorm:"column:user_id"`
		Email       string    `gorm:"column:email"`
		Name        string    `gorm:"column:name"`
		ProjectRole string    `gorm:"column:project_role"`
		CreatedAt   time.Time `gorm:"column:created_at"`
	}
	var rows []memberRow
	db.Table("project_memberships").
		Select("project_memberships.user_id, users.email, users.name, project_memberships.project_role, project_memberships.created_at").
		Joins("JOIN users ON users.id = project_memberships.user_id").
		Where("project_memberships.project_id = ?", projectID).
		Order("project_memberships.created_at ASC").
		Scan(&rows)

	out := make([]projectMemberResponse, len(rows))
	for i, r := range rows {
		out[i] = projectMemberResponse{
			UserID:      r.UserID,
			Email:       r.Email,
			Name:        r.Name,
			ProjectRole: r.ProjectRole,
			CreatedAt:   r.CreatedAt,
		}
	}

	c.JSON(http.StatusOK, gin.H{"members": out, "count": len(out)})
}

type addProjectMemberReq struct {
	UserID      string `json:"user_id" binding:"required"`
	ProjectRole string `json:"project_role"` // defaults to project_viewer
}

// handleAddProjectMember adds a tenant member to a project.
// POST /v1/projects/:id/members
func (s *Server) handleAddProjectMember(c *gin.Context) {
	_, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	tenantID, _ := middleware.GetTenantIDFromContext(c)

	projectID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}

	var req addProjectMemberReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	role := models.ProjectRoleViewer
	if req.ProjectRole == models.ProjectRoleAdmin || req.ProjectRole == models.ProjectRoleEditor {
		role = req.ProjectRole
	}

	db := s.postgresDB.GetDB()

	// Validate project belongs to tenant.
	var project models.Project
	if err := db.Where("id = ? AND tenant_id = ?", projectID, tenantID).First(&project).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	// Validate user is a member of this tenant.
	var tm models.TenantMembership
	if err := db.Where("tenant_id = ? AND user_id = ?", tenantID, req.UserID).First(&tm).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user is not a member of this tenant"})
		return
	}

	// Check if already a member.
	var existing models.ProjectMembership
	if err := db.Where("project_id = ? AND user_id = ?", projectID, req.UserID).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "user is already a member of this project"})
		return
	}

	pm := models.ProjectMembership{
		ProjectID:   uint(projectID),
		UserID:      req.UserID,
		ProjectRole: role,
	}
	if err := db.Create(&pm).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add project member"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"user_id":      req.UserID,
		"project_role": role,
	})
}

type updateProjectMemberRoleReq struct {
	ProjectRole string `json:"project_role" binding:"required"`
}

// handleUpdateProjectMemberRole updates a member's project role.
// PATCH /v1/projects/:id/members/:user_id
func (s *Server) handleUpdateProjectMemberRole(c *gin.Context) {
	_, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	tenantID, _ := middleware.GetTenantIDFromContext(c)

	projectID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}
	targetUserID := c.Param("user_id")

	var req updateProjectMemberRoleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.ProjectRole != models.ProjectRoleAdmin &&
		req.ProjectRole != models.ProjectRoleEditor &&
		req.ProjectRole != models.ProjectRoleViewer {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project_role"})
		return
	}

	db := s.postgresDB.GetDB()

	// Validate project belongs to tenant.
	var project models.Project
	if err := db.Where("id = ? AND tenant_id = ?", projectID, tenantID).First(&project).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	res := db.Model(&models.ProjectMembership{}).
		Where("project_id = ? AND user_id = ?", projectID, targetUserID).
		Update("project_role", req.ProjectRole)
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "project member not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user_id":      targetUserID,
		"project_role": req.ProjectRole,
	})
}

// handleRemoveProjectMember removes a member from a project.
// DELETE /v1/projects/:id/members/:user_id
func (s *Server) handleRemoveProjectMember(c *gin.Context) {
	_, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	tenantID, _ := middleware.GetTenantIDFromContext(c)

	projectID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}
	targetUserID := c.Param("user_id")

	db := s.postgresDB.GetDB()

	// Validate project belongs to tenant.
	var project models.Project
	if err := db.Where("id = ? AND tenant_id = ?", projectID, tenantID).First(&project).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	res := db.Where("project_id = ? AND user_id = ?", projectID, targetUserID).Delete(&models.ProjectMembership{})
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "project member not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"removed": true})
}
