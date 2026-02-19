package api

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/xiaoboyu/burnrate-ai/api-server/internal/middleware"
	"github.com/xiaoboyu/burnrate-ai/api-server/internal/models"
)

// deleteClerkUser calls the Clerk Backend API to permanently delete a user.
func (s *Server) deleteClerkUser(clerkUserID string) error {
	if s.clerkSecretKey == "" {
		return fmt.Errorf("CLERK_SECRET_KEY not configured")
	}
	req, err := http.NewRequest(http.MethodDelete, "https://api.clerk.com/v1/users/"+clerkUserID, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.clerkSecretKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("clerk returned HTTP %d", resp.StatusCode)
	}
	return nil
}

type userResponse struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

func toUserResponse(u models.User) userResponse {
	return userResponse{ID: u.ID, Email: u.Email, Name: u.Name, Role: u.Role, Status: u.Status, CreatedAt: u.CreatedAt}
}

// handleListUsers returns all users in the caller's tenant.
// GET /v1/admin/users
func (s *Server) handleListUsers(c *gin.Context) {
	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var users []models.User
	if err := s.postgresDB.GetDB().
		Where("tenant_id = ?", caller.TenantID).
		Order("created_at ASC").
		Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch users"})
		return
	}

	out := make([]userResponse, len(users))
	for i, u := range users {
		out[i] = toUserResponse(u)
	}
	c.JSON(http.StatusOK, gin.H{"users": out, "total": len(out)})
}

// ── Invite ───────────────────────────────────────────────────────────────────

type inviteUserReq struct {
	Email string `json:"email" binding:"required,email"`
	Name  string `json:"name"`
	Role  string `json:"role"` // viewer | editor; defaults to viewer
}

// handleInviteUser creates a pending user in the tenant.
// The invitee activates on first sign-in via auth/sync (email matched).
// POST /v1/admin/users/invite
func (s *Server) handleInviteUser(c *gin.Context) {
	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req inviteUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	role := models.RoleViewer
	if req.Role == models.RoleEditor {
		role = models.RoleEditor
	} else if req.Role != "" && req.Role != models.RoleViewer {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_role",
			"message": "Can only invite users as 'viewer' or 'editor'",
		})
		return
	}

	db := s.postgresDB.GetDB()

	// Enforce plan member limit.
	var tenant models.Tenant
	db.First(&tenant, caller.TenantID)
	lim := models.GetPlanLimits(tenant.Plan)
	if lim.MaxMembers != -1 {
		var memberCount int64
		db.Model(&models.User{}).Where("tenant_id = ?", caller.TenantID).Count(&memberCount)
		if int(memberCount) >= lim.MaxMembers {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error":   "plan_limit_reached",
				"message": fmt.Sprintf("Your %s plan allows up to %d team member(s). Upgrade to add more.", tenant.Plan, lim.MaxMembers),
				"limit":   lim.MaxMembers,
				"current": memberCount,
				"plan":    tenant.Plan,
			})
			return
		}
	}

	var existing models.User
	if err := db.Where("email = ?", req.Email).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{
			"error":   "user_exists",
			"message": "A user with this email already exists",
		})
		return
	}

	invited := models.User{
		ID:        fmt.Sprintf("pending_%s_%d", req.Email, time.Now().UnixNano()),
		TenantID:  caller.TenantID,
		Email:     req.Email,
		Name:      req.Name,
		Role:      role,
		Status:    models.StatusPending,
		CreatedAt: time.Now(),
	}
	if err := db.Create(&invited).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to invite user"})
		return
	}

	log.Printf("user invited: email=%s role=%s by=%s tenant_id=%d", req.Email, role, caller.Email, caller.TenantID)
	c.JSON(http.StatusCreated, gin.H{
		"message":        "User invited. They will join your tenant when they sign up.",
		"signup_url":     "https://burnrate-ai-weld.vercel.app/sign-up",
		"user":           toUserResponse(invited),
	})
}

// ── Role management (admin+) ─────────────────────────────────────────────────

type updateRoleReq struct {
	Role string `json:"role" binding:"required"`
}

// handleUpdateUserRole changes a user's role (viewer/editor only for admins).
// PATCH /v1/admin/users/:user_id/role
func (s *Server) handleUpdateUserRole(c *gin.Context) {
	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req updateRoleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Role != models.RoleViewer && req.Role != models.RoleEditor {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_role",
			"message": "Can only set role to 'viewer' or 'editor'. Use owner endpoints for admin promotion.",
		})
		return
	}

	targetID := c.Param("user_id")
	if targetID == caller.ID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot change your own role"})
		return
	}

	var target models.User
	if err := s.postgresDB.GetDB().Where("id = ? AND tenant_id = ?", targetID, caller.TenantID).First(&target).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	if target.Role == models.RoleOwner {
		c.JSON(http.StatusForbidden, gin.H{"error": "cannot change the owner's role"})
		return
	}
	if target.Role == models.RoleAdmin && caller.Role != models.RoleOwner {
		c.JSON(http.StatusForbidden, gin.H{"error": "only the owner can change admin roles"})
		return
	}

	if err := s.postgresDB.GetDB().Model(&models.User{}).
		Where("id = ?", targetID).Update("role", req.Role).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update role"})
		return
	}
	target.Role = req.Role
	c.JSON(http.StatusOK, gin.H{"message": "Role updated successfully", "user": toUserResponse(target)})
}

// handleSuspendUser suspends a user account.
// PATCH /v1/admin/users/:user_id/suspend
func (s *Server) handleSuspendUser(c *gin.Context) {
	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	targetID := c.Param("user_id")
	if targetID == caller.ID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot suspend yourself"})
		return
	}

	var target models.User
	if err := s.postgresDB.GetDB().Where("id = ? AND tenant_id = ?", targetID, caller.TenantID).First(&target).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	if target.Role == models.RoleOwner {
		c.JSON(http.StatusForbidden, gin.H{"error": "cannot suspend the tenant owner"})
		return
	}
	if target.Role == models.RoleAdmin && caller.Role != models.RoleOwner {
		c.JSON(http.StatusForbidden, gin.H{"error": "only the owner can suspend admins"})
		return
	}

	if err := s.postgresDB.GetDB().Model(&models.User{}).
		Where("id = ?", targetID).Update("status", models.StatusSuspended).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to suspend user"})
		return
	}
	target.Status = models.StatusSuspended
	c.JSON(http.StatusOK, gin.H{"message": "User suspended successfully", "user": toUserResponse(target)})
}

// handleUnsuspendUser reactivates a suspended user account.
// PATCH /v1/admin/users/:user_id/unsuspend
func (s *Server) handleUnsuspendUser(c *gin.Context) {
	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	targetID := c.Param("user_id")
	var target models.User
	if err := s.postgresDB.GetDB().Where("id = ? AND tenant_id = ?", targetID, caller.TenantID).First(&target).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	if err := s.postgresDB.GetDB().Model(&models.User{}).
		Where("id = ?", targetID).Update("status", models.StatusActive).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to unsuspend user"})
		return
	}
	target.Status = models.StatusActive
	c.JSON(http.StatusOK, gin.H{"message": "User unsuspended successfully", "user": toUserResponse(target)})
}

// handleRemoveUser deletes a user from the tenant.
// DELETE /v1/admin/users/:user_id
func (s *Server) handleRemoveUser(c *gin.Context) {
	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	targetID := c.Param("user_id")
	if targetID == caller.ID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot remove yourself"})
		return
	}

	var target models.User
	if err := s.postgresDB.GetDB().Where("id = ? AND tenant_id = ?", targetID, caller.TenantID).First(&target).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	if target.Role == models.RoleOwner {
		c.JSON(http.StatusForbidden, gin.H{"error": "cannot remove the tenant owner. Transfer ownership first."})
		return
	}
	if target.Role == models.RoleAdmin && caller.Role != models.RoleOwner {
		c.JSON(http.StatusForbidden, gin.H{"error": "only the owner can remove admins"})
		return
	}

	if err := s.postgresDB.GetDB().Delete(&models.User{}, "id = ?", targetID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove user"})
		return
	}

	if err := s.deleteClerkUser(target.ID); err != nil {
		log.Printf("warning: user %s removed from DB but Clerk deletion failed: %v", target.Email, err)
	}

	log.Printf("user removed: %s by %s", target.Email, caller.Email)
	c.JSON(http.StatusOK, gin.H{"message": "User removed successfully"})
}

// ── Owner-only endpoints ─────────────────────────────────────────────────────

type tenantSettingsResponse struct {
	TenantID   uint               `json:"tenant_id"`
	Name       string             `json:"name"`
	Plan       string             `json:"plan"`
	MaxAPIKeys int                `json:"max_api_keys"`
	PlanLimits models.PlanLimits  `json:"plan_limits"`
}

// handleGetTenantSettings returns the current tenant settings including plan info.
// GET /v1/owner/settings
func (s *Server) handleGetTenantSettings(c *gin.Context) {
	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var tenant models.Tenant
	if err := s.postgresDB.GetDB().First(&tenant, caller.TenantID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch tenant"})
		return
	}
	c.JSON(http.StatusOK, tenantSettingsResponse{
		TenantID:   tenant.ID,
		Name:       tenant.Name,
		Plan:       tenant.Plan,
		MaxAPIKeys: tenant.MaxAPIKeys,
		PlanLimits: models.GetPlanLimits(tenant.Plan),
	})
}

type updateTenantSettingsReq struct {
	MaxAPIKeys *int `json:"max_api_keys"`
}

// handleUpdateTenantSettings updates owner-controlled tenant settings.
// PATCH /v1/owner/settings
func (s *Server) handleUpdateTenantSettings(c *gin.Context) {
	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var req updateTenantSettingsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.MaxAPIKeys == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no settings provided"})
		return
	}
	if *req.MaxAPIKeys < 1 || *req.MaxAPIKeys > 1000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "max_api_keys must be between 1 and 1000"})
		return
	}
	var tenant models.Tenant
	if err := s.postgresDB.GetDB().First(&tenant, caller.TenantID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch tenant"})
		return
	}
	// Plans with a fixed limit (non -1) cannot be overridden.
	planLim := models.GetPlanLimits(tenant.Plan)
	if planLim.MaxAPIKeys != -1 && *req.MaxAPIKeys > planLim.MaxAPIKeys {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "plan_limit_exceeded",
			"message":    fmt.Sprintf("Your %s plan allows a maximum of %d API key(s). Upgrade to set a higher limit.", tenant.Plan, planLim.MaxAPIKeys),
			"plan_limit": planLim.MaxAPIKeys,
			"plan":       tenant.Plan,
		})
		return
	}
	tenant.MaxAPIKeys = *req.MaxAPIKeys
	if err := s.postgresDB.GetDB().Model(&tenant).Update("max_api_keys", *req.MaxAPIKeys).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update settings"})
		return
	}
	c.JSON(http.StatusOK, tenantSettingsResponse{
		TenantID:   tenant.ID,
		Name:       tenant.Name,
		Plan:       tenant.Plan,
		MaxAPIKeys: tenant.MaxAPIKeys,
		PlanLimits: models.GetPlanLimits(tenant.Plan),
	})
}

// handlePromoteAdmin promotes a viewer or editor to admin (owner only).
// POST /v1/owner/users/:user_id/promote-admin
func (s *Server) handlePromoteAdmin(c *gin.Context) {
	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	targetID := c.Param("user_id")
	if targetID == caller.ID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot promote yourself"})
		return
	}

	var target models.User
	if err := s.postgresDB.GetDB().Where("id = ? AND tenant_id = ?", targetID, caller.TenantID).First(&target).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	if target.Role == models.RoleOwner {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user is already the owner"})
		return
	}
	if target.Role == models.RoleAdmin {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user is already an admin"})
		return
	}

	if err := s.postgresDB.GetDB().Model(&models.User{}).
		Where("id = ?", targetID).Update("role", models.RoleAdmin).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to promote user"})
		return
	}
	target.Role = models.RoleAdmin
	log.Printf("user promoted to admin: %s by %s", target.Email, caller.Email)
	c.JSON(http.StatusOK, gin.H{"message": "User promoted to admin successfully", "user": toUserResponse(target)})
}

// handleDemoteAdmin demotes an admin to editor (owner only).
// DELETE /v1/owner/users/:user_id/demote-admin
func (s *Server) handleDemoteAdmin(c *gin.Context) {
	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	targetID := c.Param("user_id")
	var target models.User
	if err := s.postgresDB.GetDB().Where("id = ? AND tenant_id = ?", targetID, caller.TenantID).First(&target).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	if target.Role != models.RoleAdmin {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user is not an admin"})
		return
	}

	if err := s.postgresDB.GetDB().Model(&models.User{}).
		Where("id = ?", targetID).Update("role", models.RoleEditor).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to demote admin"})
		return
	}
	target.Role = models.RoleEditor
	log.Printf("admin demoted to editor: %s by %s", target.Email, caller.Email)
	c.JSON(http.StatusOK, gin.H{"message": "Admin demoted to editor successfully", "user": toUserResponse(target)})
}

// handleTransferOwnership transfers ownership to another active tenant member.
// POST /v1/owner/transfer-ownership
func (s *Server) handleTransferOwnership(c *gin.Context) {
	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req struct {
		NewOwnerID string `json:"new_owner_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.NewOwnerID == caller.ID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "you are already the owner"})
		return
	}

	var newOwner models.User
	if err := s.postgresDB.GetDB().Where("id = ? AND tenant_id = ?", req.NewOwnerID, caller.TenantID).First(&newOwner).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "target user not found in your tenant"})
		return
	}

	if newOwner.Status != models.StatusActive {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot transfer ownership to a suspended user"})
		return
	}

	tx := s.postgresDB.GetDB().Begin()
	if err := tx.Model(&models.User{}).Where("id = ?", caller.ID).Update("role", models.RoleAdmin).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to transfer ownership"})
		return
	}
	if err := tx.Model(&models.User{}).Where("id = ?", newOwner.ID).Update("role", models.RoleOwner).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to transfer ownership"})
		return
	}
	tx.Commit()

	log.Printf("ownership transferred from %s to %s", caller.Email, newOwner.Email)
	c.JSON(http.StatusOK, gin.H{
		"message":   "Ownership transferred successfully",
		"new_owner": newOwner.Email,
	})
}

// ── Plan change ───────────────────────────────────────────────────────────────

type changePlanReq struct {
	Plan string `json:"plan" binding:"required"`
}

// applyPlanChange enforces downgrade limits and updates tenant.plan + tenant.max_api_keys.
// Returns (http status, error body) on failure, or (0, nil) on success.
func (s *Server) applyPlanChange(tenantID uint, newPlan string) (int, gin.H) {
	db := s.postgresDB.GetDB()

	if !models.ValidPlan(newPlan) {
		return http.StatusBadRequest, gin.H{
			"error":   "invalid_plan",
			"message": fmt.Sprintf("Unknown plan %q. Valid plans: free, pro, team, business.", newPlan),
		}
	}

	var tenant models.Tenant
	if err := db.First(&tenant, tenantID).Error; err != nil {
		return http.StatusInternalServerError, gin.H{"error": "failed to fetch tenant"}
	}

	if tenant.Plan == newPlan {
		return http.StatusBadRequest, gin.H{
			"error":   "no_change",
			"message": fmt.Sprintf("Tenant is already on the %s plan.", newPlan),
		}
	}

	newLimits := models.GetPlanLimits(newPlan)

	// ── Downgrade enforcement ────────────────────────────────────────────────
	// API key count
	if newLimits.MaxAPIKeys != -1 {
		var activeKeys int64
		db.Model(&models.APIKey{}).
			Where("tenant_id = ? AND revoked = false AND (expires_at IS NULL OR expires_at > NOW())", tenantID).
			Count(&activeKeys)
		if int(activeKeys) > newLimits.MaxAPIKeys {
			return http.StatusUnprocessableEntity, gin.H{
				"error":        "downgrade_blocked",
				"reason":       "api_keys_exceed_limit",
				"message":      fmt.Sprintf("The %s plan allows %d API key(s), but this tenant has %d active key(s). Revoke %d key(s) before downgrading.", newPlan, newLimits.MaxAPIKeys, activeKeys, int(activeKeys)-newLimits.MaxAPIKeys),
				"limit":        newLimits.MaxAPIKeys,
				"active_count": activeKeys,
			}
		}
	}

	// Member count (all users regardless of status, matching invite enforcement)
	if newLimits.MaxMembers != -1 {
		var memberCount int64
		db.Model(&models.User{}).Where("tenant_id = ?", tenantID).Count(&memberCount)
		if int(memberCount) > newLimits.MaxMembers {
			return http.StatusUnprocessableEntity, gin.H{
				"error":        "downgrade_blocked",
				"reason":       "members_exceed_limit",
				"message":      fmt.Sprintf("The %s plan allows %d member(s), but this tenant has %d. Remove %d member(s) before downgrading.", newPlan, newLimits.MaxMembers, memberCount, int(memberCount)-newLimits.MaxMembers),
				"limit":        newLimits.MaxMembers,
				"member_count": memberCount,
			}
		}
	}

	// ── Apply ────────────────────────────────────────────────────────────────
	// max_api_keys tracks the plan ceiling; reset it to the new plan's limit
	// (-1 = unlimited stored as -1 in DB, which existing enforcement already handles).
	newMaxAPIKeys := newLimits.MaxAPIKeys
	if err := db.Model(&tenant).Updates(map[string]any{
		"plan":         newPlan,
		"max_api_keys": newMaxAPIKeys,
	}).Error; err != nil {
		return http.StatusInternalServerError, gin.H{"error": "failed to update plan"}
	}

	log.Printf("tenant %d plan changed: %s → %s", tenantID, tenant.Plan, newPlan)
	return 0, nil
}

// handleChangePlan lets the tenant owner change their own plan.
// PATCH /v1/owner/plan
func (s *Server) handleChangePlan(c *gin.Context) {
	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req changePlanReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if status, body := s.applyPlanChange(caller.TenantID, req.Plan); status != 0 {
		c.JSON(status, body)
		return
	}

	// Return updated settings in the same shape as GET /v1/owner/settings.
	var tenant models.Tenant
	s.postgresDB.GetDB().First(&tenant, caller.TenantID)
	c.JSON(http.StatusOK, tenantSettingsResponse{
		TenantID:   tenant.ID,
		Name:       tenant.Name,
		Plan:       tenant.Plan,
		MaxAPIKeys: tenant.MaxAPIKeys,
		PlanLimits: models.GetPlanLimits(tenant.Plan),
	})
}

// handleAdminChangeTenantPlan lets a platform operator change any tenant's plan.
// PATCH /v1/internal/tenants/:tenant_id/plan
func (s *Server) handleAdminChangeTenantPlan(c *gin.Context) {
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

	// Verify tenant exists.
	var tenant models.Tenant
	if err := s.postgresDB.GetDB().First(&tenant, tenantID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tenant not found"})
		return
	}

	if status, body := s.applyPlanChange(tenantID, req.Plan); status != 0 {
		c.JSON(status, body)
		return
	}

	s.postgresDB.GetDB().First(&tenant, tenantID)
	c.JSON(http.StatusOK, tenantSettingsResponse{
		TenantID:   tenant.ID,
		Name:       tenant.Name,
		Plan:       tenant.Plan,
		MaxAPIKeys: tenant.MaxAPIKeys,
		PlanLimits: models.GetPlanLimits(tenant.Plan),
	})
}
