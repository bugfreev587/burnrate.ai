package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/xiaoboyu/tokengate/api-server/internal/middleware"
	"github.com/xiaoboyu/tokengate/api-server/internal/models"
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
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("clerk returned HTTP %d: %s", resp.StatusCode, string(body))
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

func toUserResponse(u models.User, orgRole string, membershipStatus string) userResponse {
	return userResponse{
		ID:        u.ID,
		Email:     u.Email,
		Name:      u.Name,
		Role:      orgRole,
		Status:    membershipStatus,
		CreatedAt: u.CreatedAt,
	}
}

type onboardingHintsResponse struct {
	DismissedIntegrationHint bool `json:"dismissed_integration_hint"`
	DismissedAvatarHint      bool `json:"dismissed_avatar_hint"`
}

type updateOnboardingHintsReq struct {
	DismissedIntegrationHint *bool `json:"dismissed_integration_hint"`
	DismissedAvatarHint      *bool `json:"dismissed_avatar_hint"`
}

// handleGetOnboardingHints returns per-user onboarding hint dismissal flags.
// GET /v1/user/onboarding-hints
func (s *Server) handleGetOnboardingHints(c *gin.Context) {
	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	c.JSON(http.StatusOK, onboardingHintsResponse{
		DismissedIntegrationHint: caller.DismissedIntegrationHint,
		DismissedAvatarHint:      caller.DismissedAvatarHint,
	})
}

// handleUpdateOnboardingHints updates per-user onboarding hint dismissal flags.
// PATCH /v1/user/onboarding-hints
func (s *Server) handleUpdateOnboardingHints(c *gin.Context) {
	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req updateOnboardingHintsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]interface{}{}
	if req.DismissedIntegrationHint != nil {
		updates["dismissed_integration_hint"] = *req.DismissedIntegrationHint
	}
	if req.DismissedAvatarHint != nil {
		updates["dismissed_avatar_hint"] = *req.DismissedAvatarHint
	}
	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no fields to update"})
		return
	}

	db := s.postgresDB.GetDB()
	if err := db.Model(&models.User{}).Where("id = ?", caller.ID).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update onboarding hints"})
		return
	}

	var updated models.User
	if err := db.Where("id = ?", caller.ID).First(&updated).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch updated user"})
		return
	}

	c.JSON(http.StatusOK, onboardingHintsResponse{
		DismissedIntegrationHint: updated.DismissedIntegrationHint,
		DismissedAvatarHint:      updated.DismissedAvatarHint,
	})
}

// handleListUsers returns all users in the caller's tenant via tenant_memberships.
// GET /v1/admin/users
func (s *Server) handleListUsers(c *gin.Context) {
	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}

	db := s.postgresDB.GetDB()

	// Join tenant_memberships with users to get all members in this tenant.
	type memberRow struct {
		models.User
		OrgRole          string `gorm:"column:org_role"`
		MembershipStatus string `gorm:"column:membership_status"`
	}
	var rows []memberRow
	if err := db.
		Table("users").
		Select("users.*, tenant_memberships.org_role, tenant_memberships.status AS membership_status").
		Joins("JOIN tenant_memberships ON tenant_memberships.user_id = users.id").
		Where("tenant_memberships.tenant_id = ?", tenantID).
		Order("users.created_at ASC").
		Scan(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch users"})
		return
	}

	out := make([]userResponse, len(rows))
	for i, r := range rows {
		out[i] = toUserResponse(r.User, r.OrgRole, r.MembershipStatus)
	}

	var tenant models.Tenant
	db.First(&tenant, tenantID)
	planLim := models.GetPlanLimits(tenant.Plan)

	// Member count from memberships.
	var memberCount int64
	db.Model(&models.TenantMembership{}).Where("tenant_id = ?", tenantID).Count(&memberCount)

	// member_limit is null for unlimited plans (-1), matching the api_keys response convention.
	var memberLimit *int
	if planLim.MaxMembers != -1 {
		memberLimit = &planLim.MaxMembers
	}

	_ = caller // caller used for auth; tenantID drives the query
	c.JSON(http.StatusOK, gin.H{
		"users":        out,
		"total":        len(out),
		"plan":         tenant.Plan,
		"member_limit": memberLimit,
	})
}

// ── Invite ───────────────────────────────────────────────────────────────────

type inviteUserReq struct {
	Email string `json:"email" binding:"required,email"`
	Name  string `json:"name"`
	Role  string `json:"role"` // viewer | editor; defaults to viewer
}

func roleLabel(role string) string {
	switch role {
	case models.RoleOwner:
		return "Owner"
	case models.RoleAdmin:
		return "Admin"
	case models.RoleEditor:
		return "Editor"
	default:
		return "Viewer"
	}
}

func markInvitationNotificationsResolved(db *gorm.DB, userID string, tenantID uint, result string) {
	var rows []models.UserNotification
	if err := db.Where("user_id = ? AND tenant_id = ? AND type = ?", userID, tenantID, models.EventTeamInvitation).
		Find(&rows).Error; err != nil {
		slog.Warn("invitation_notification_load_failed", "user_id", userID, "tenant_id", tenantID, "error", err)
		return
	}

	now := time.Now()
	for _, n := range rows {
		payload := map[string]interface{}{}
		if n.Payload != "" {
			_ = json.Unmarshal([]byte(n.Payload), &payload)
		}
		payload["invitation_status"] = result
		payloadJSON, _ := json.Marshal(payload)

		resolvedBody := "This invitation has been processed."
		if result == "accepted" {
			resolvedBody = "This invitation has been accepted."
		} else if result == "denied" {
			resolvedBody = "This invitation has been denied."
		}

		if err := db.Model(&models.UserNotification{}).
			Where("id = ?", n.ID).
			Updates(map[string]interface{}{
				"payload": string(payloadJSON),
				"body":    resolvedBody,
				"status":  models.UserNotificationStatusRead,
				"read_at": &now,
			}).Error; err != nil {
			slog.Warn("invitation_notification_update_failed", "notification_id", n.ID, "error", err)
		}
	}
}

// handleInviteUser creates a pending user + TenantMembership in the tenant.
// If the user already exists (e.g. in another tenant), a new membership is created.
// POST /v1/admin/users/invite
func (s *Server) handleInviteUser(c *gin.Context) {
	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
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

	// Enforce plan member limit using membership count.
	var tenant models.Tenant
	db.First(&tenant, tenantID)
	lim := models.GetPlanLimits(tenant.Plan)
	if lim.MaxMembers != -1 {
		var memberCount int64
		db.Model(&models.TenantMembership{}).Where("tenant_id = ?", tenantID).Count(&memberCount)
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

	// Check if user already has a membership in this tenant.
	var existing models.User
	userExists := db.Where("email = ?", req.Email).First(&existing).Error == nil

	if userExists {
		// Check if they already have a membership in this tenant.
		var existingMembership models.TenantMembership
		if err := db.Where("tenant_id = ? AND user_id = ?", tenantID, existing.ID).First(&existingMembership).Error; err == nil {
			c.JSON(http.StatusConflict, gin.H{
				"error":   "user_exists",
				"message": "This user is already a member of your tenant",
			})
			return
		}
		// User exists in another tenant; create membership for this tenant.
		membership := models.TenantMembership{
			TenantID:  tenantID,
			UserID:    existing.ID,
			OrgRole:   role,
			Status:    models.StatusPending,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := db.Create(&membership).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create membership"})
			return
		}

		payload, _ := json.Marshal(gin.H{
			"tenant_id":         tenantID,
			"tenant_name":       tenant.Name,
			"invited_role":      role,
			"invited_by":        caller.Email,
			"invitation_status": "pending",
		})
		title := fmt.Sprintf("%s invited you to %s", caller.Email, tenant.Name)
		body := fmt.Sprintf("Role: %s. Choose Accept, Deny, or Decide later.", roleLabel(role))
		_ = db.Create(&models.UserNotification{
			UserID:   existing.ID,
			TenantID: &tenantID,
			Type:     models.EventTeamInvitation,
			Title:    title,
			Body:     body,
			Payload:  string(payload),
			Status:   models.UserNotificationStatusUnread,
		}).Error
		if s.notifWorker != nil {
			if err := s.notifWorker.SendUserNotification(c.Request.Context(), existing.ID, models.EventTeamInvitation, title, body, string(payload)); err != nil {
				slog.Warn("invite_personal_channel_dispatch_failed", "user_id", existing.ID, "tenant_id", tenantID, "error", err)
			}
		}

		s.recordAuditEvent(c, models.AuditMemberInvited, "membership", existing.ID, AuditOpts{
			Category: models.AuditCategoryTeam,
			AfterState: map[string]interface{}{
				"email": req.Email,
				"role":  role,
			},
		})

		slog.Info("user_invited", "email", req.Email, "role", role, "by", caller.Email, "tenant_id", tenantID)
		c.JSON(http.StatusCreated, gin.H{
			"message":    "User invited. They will join your tenant when they accept.",
			"signup_url": "https://app.tokengate.to/sign-up",
			"user":       toUserResponse(existing, role, models.StatusPending),
		})
		return
	}

	// User does not exist; create a pending User record + TenantMembership.
	invited := models.User{
		ID:        fmt.Sprintf("pending_%s_%d", req.Email, time.Now().UnixNano()),
		Email:     req.Email,
		Name:      req.Name,
		Status:    models.StatusPending,
		CreatedAt: time.Now(),
	}
	if err := db.Create(&invited).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to invite user"})
		return
	}

	membership := models.TenantMembership{
		TenantID:  tenantID,
		UserID:    invited.ID,
		OrgRole:   role,
		Status:    models.StatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := db.Create(&membership).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create membership"})
		return
	}

	s.recordAuditEvent(c, models.AuditMemberInvited, "membership", invited.ID, AuditOpts{
		Category: models.AuditCategoryTeam,
		AfterState: map[string]interface{}{
			"email": req.Email,
			"role":  role,
		},
	})

	slog.Info("user_invited", "email", req.Email, "role", role, "by", caller.Email, "tenant_id", tenantID)
	c.JSON(http.StatusCreated, gin.H{
		"message":    "User invited. They will join your tenant when they sign up.",
		"signup_url": "https://app.tokengate.to/sign-up",
		"user":       toUserResponse(invited, role, models.StatusPending),
	})
}

// POST /v1/user/invitations/:tenant_id/accept
func (s *Server) handleAcceptInvitation(c *gin.Context) {
	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var tenantID uint
	if _, err := fmt.Sscanf(c.Param("tenant_id"), "%d", &tenantID); err != nil || tenantID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tenant_id"})
		return
	}

	db := s.postgresDB.GetDB()
	var m models.TenantMembership
	if err := db.Where("tenant_id = ? AND user_id = ?", tenantID, caller.ID).First(&m).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "invitation not found"})
		return
	}
	if m.Status != models.StatusPending {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invitation is not pending"})
		return
	}
	if err := db.Model(&m).Updates(map[string]interface{}{
		"status":     models.StatusActive,
		"updated_at": time.Now(),
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to accept invitation"})
		return
	}

	markInvitationNotificationsResolved(db, caller.ID, tenantID, "accepted")

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// POST /v1/user/invitations/:tenant_id/deny
func (s *Server) handleDenyInvitation(c *gin.Context) {
	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var tenantID uint
	if _, err := fmt.Sscanf(c.Param("tenant_id"), "%d", &tenantID); err != nil || tenantID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tenant_id"})
		return
	}

	db := s.postgresDB.GetDB()
	var m models.TenantMembership
	if err := db.Where("tenant_id = ? AND user_id = ?", tenantID, caller.ID).First(&m).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "invitation not found"})
		return
	}
	if m.Status != models.StatusPending {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invitation is not pending"})
		return
	}
	if err := db.Delete(&m).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to deny invitation"})
		return
	}

	markInvitationNotificationsResolved(db, caller.ID, tenantID, "denied")

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ── Role management (admin+) ─────────────────────────────────────────────────

type updateRoleReq struct {
	Role string `json:"role" binding:"required"`
}

// handleUpdateUserRole changes a user's org_role in tenant_memberships (viewer/editor only for admins).
// PATCH /v1/admin/users/:user_id/role
func (s *Server) handleUpdateUserRole(c *gin.Context) {
	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}
	callerRole := middleware.GetOrgRoleFromContext(c)

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

	db := s.postgresDB.GetDB()

	// Look up target's membership in this tenant.
	var targetMembership models.TenantMembership
	if err := db.Where("tenant_id = ? AND user_id = ?", tenantID, targetID).First(&targetMembership).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	if targetMembership.OrgRole == models.RoleOwner {
		c.JSON(http.StatusForbidden, gin.H{"error": "cannot change the owner's role"})
		return
	}
	if targetMembership.OrgRole == models.RoleAdmin && callerRole != models.RoleOwner {
		c.JSON(http.StatusForbidden, gin.H{"error": "only the owner can change admin roles"})
		return
	}

	oldRole := targetMembership.OrgRole

	if err := db.Model(&models.TenantMembership{}).
		Where("tenant_id = ? AND user_id = ?", tenantID, targetID).
		Update("org_role", req.Role).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update role"})
		return
	}

	s.recordAuditEvent(c, models.AuditMemberRoleChanged, "membership", targetID, AuditOpts{
		Category: models.AuditCategoryTeam,
		BeforeState: map[string]interface{}{
			"role": oldRole,
		},
		AfterState: map[string]interface{}{
			"role": req.Role,
		},
	})

	var target models.User
	db.Where("id = ?", targetID).First(&target)
	c.JSON(http.StatusOK, gin.H{"message": "Role updated successfully", "user": toUserResponse(target, req.Role, targetMembership.Status)})
}

// handleSuspendUser suspends a user's membership in the tenant.
// PATCH /v1/admin/users/:user_id/suspend
func (s *Server) handleSuspendUser(c *gin.Context) {
	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}
	callerRole := middleware.GetOrgRoleFromContext(c)

	targetID := c.Param("user_id")
	if targetID == caller.ID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot suspend yourself"})
		return
	}

	db := s.postgresDB.GetDB()

	var targetMembership models.TenantMembership
	if err := db.Where("tenant_id = ? AND user_id = ?", tenantID, targetID).First(&targetMembership).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	if targetMembership.OrgRole == models.RoleOwner {
		c.JSON(http.StatusForbidden, gin.H{"error": "cannot suspend the tenant owner"})
		return
	}
	if targetMembership.OrgRole == models.RoleAdmin && callerRole != models.RoleOwner {
		c.JSON(http.StatusForbidden, gin.H{"error": "only the owner can suspend admins"})
		return
	}

	if err := db.Model(&models.TenantMembership{}).
		Where("tenant_id = ? AND user_id = ?", tenantID, targetID).
		Update("status", models.StatusSuspended).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to suspend user"})
		return
	}

	s.recordAuditEvent(c, models.AuditMemberSuspended, "membership", targetID, AuditOpts{
		Category: models.AuditCategoryTeam,
	})

	var target models.User
	db.Where("id = ?", targetID).First(&target)
	c.JSON(http.StatusOK, gin.H{"message": "User suspended successfully", "user": toUserResponse(target, targetMembership.OrgRole, models.StatusSuspended)})
}

// handleUnsuspendUser reactivates a suspended user's membership in the tenant.
// PATCH /v1/admin/users/:user_id/unsuspend
func (s *Server) handleUnsuspendUser(c *gin.Context) {
	_, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}

	targetID := c.Param("user_id")
	db := s.postgresDB.GetDB()

	var targetMembership models.TenantMembership
	if err := db.Where("tenant_id = ? AND user_id = ?", tenantID, targetID).First(&targetMembership).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	if err := db.Model(&models.TenantMembership{}).
		Where("tenant_id = ? AND user_id = ?", tenantID, targetID).
		Update("status", models.StatusActive).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to unsuspend user"})
		return
	}

	s.recordAuditEvent(c, models.AuditMemberUnsuspended, "membership", targetID, AuditOpts{
		Category: models.AuditCategoryTeam,
	})

	var target models.User
	db.Where("id = ?", targetID).First(&target)
	c.JSON(http.StatusOK, gin.H{"message": "User unsuspended successfully", "user": toUserResponse(target, targetMembership.OrgRole, models.StatusActive)})
}

// handleRemoveUser removes a user from the tenant by deleting their TenantMembership
// and any ProjectMemberships for this tenant. Only deletes the User record if no
// other TenantMemberships remain.
// DELETE /v1/admin/users/:user_id
func (s *Server) handleRemoveUser(c *gin.Context) {
	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}
	callerRole := middleware.GetOrgRoleFromContext(c)

	targetID := c.Param("user_id")
	if targetID == caller.ID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot remove yourself"})
		return
	}

	db := s.postgresDB.GetDB()

	var targetMembership models.TenantMembership
	if err := db.Where("tenant_id = ? AND user_id = ?", tenantID, targetID).First(&targetMembership).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	if targetMembership.OrgRole == models.RoleOwner {
		c.JSON(http.StatusForbidden, gin.H{"error": "cannot remove the tenant owner. Transfer ownership first."})
		return
	}
	if targetMembership.OrgRole == models.RoleAdmin && callerRole != models.RoleOwner {
		c.JSON(http.StatusForbidden, gin.H{"error": "only the owner can remove admins"})
		return
	}

	var target models.User
	db.Where("id = ?", targetID).First(&target)

	tx := db.Begin()

	// Delete ProjectMemberships for projects in this tenant.
	if err := tx.Exec(
		"DELETE FROM project_memberships WHERE user_id = ? AND project_id IN (SELECT id FROM projects WHERE tenant_id = ?)",
		targetID, tenantID,
	).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove project memberships"})
		return
	}

	// Delete TenantMembership.
	if err := tx.Where("tenant_id = ? AND user_id = ?", tenantID, targetID).
		Delete(&models.TenantMembership{}).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove user"})
		return
	}

	// Check if the user has any remaining TenantMemberships.
	var remainingCount int64
	tx.Model(&models.TenantMembership{}).Where("user_id = ?", targetID).Count(&remainingCount)

	if remainingCount == 0 {
		// No other memberships remain; delete the User record.
		if err := tx.Delete(&models.User{}, "id = ?", targetID).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove user"})
			return
		}
		// Also clean up the Clerk user.
		if err := s.deleteClerkUser(target.ID); err != nil {
			slog.Warn("clerk_user_deletion_failed", "email", target.Email, "error", err)
		}
	}

	tx.Commit()

	s.recordAuditEvent(c, models.AuditMemberRemoved, "membership", targetID, AuditOpts{
		Category: models.AuditCategoryTeam,
		BeforeState: map[string]interface{}{
			"email": target.Email,
			"role":  targetMembership.OrgRole,
		},
	})

	slog.Info("user_removed", "email", target.Email, "by", caller.Email, "tenant_id", tenantID)
	c.JSON(http.StatusOK, gin.H{"message": "User removed successfully"})
}

// ── Owner-only endpoints ─────────────────────────────────────────────────────

type tenantSettingsResponse struct {
	TenantID   uint              `json:"tenant_id"`
	Name       string            `json:"name"`
	Plan       string            `json:"plan"`
	PlanLimits models.PlanLimits `json:"plan_limits"`
}

// handleGetTenantSettings returns the current tenant settings including plan info.
// GET /v1/owner/settings
func (s *Server) handleGetTenantSettings(c *gin.Context) {
	_, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}

	var tenant models.Tenant
	if err := s.postgresDB.GetDB().First(&tenant, tenantID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch tenant"})
		return
	}

	// Self-heal: if Stripe's actual plan differs from DB (e.g. webhook was
	// missed or a previous update failed), correct it on read.
	if s.stripeSvc.IsConfigured() && tenant.StripeSubscriptionID != "" && tenant.PendingPlan == "" {
		if subInfo, err := s.stripeSvc.GetSubscription(c.Request.Context(), tenantID); err != nil {
			slog.Error("settings_plan_sync_fetch_failed", "tenant_id", tenantID, "error", err)
		} else if subInfo != nil && subInfo.DetectedPlan != "" && subInfo.DetectedPlan != tenant.Plan {
			if err := s.postgresDB.GetDB().Model(&tenant).Updates(map[string]any{
				"plan": subInfo.DetectedPlan,
			}).Error; err != nil {
				slog.Error("settings_plan_sync_failed", "tenant_id", tenantID, "error", err)
			} else {
				slog.Info("settings_plan_synced_from_stripe",
					"tenant_id", tenantID,
					"old_plan", tenant.Plan,
					"new_plan", subInfo.DetectedPlan,
				)
				tenant.Plan = subInfo.DetectedPlan
			}
		}
	}

	c.JSON(http.StatusOK, tenantSettingsResponse{
		TenantID:   tenant.ID,
		Name:       tenant.Name,
		Plan:       tenant.Plan,
		PlanLimits: models.GetPlanLimits(tenant.Plan),
	})
}

type updateTenantSettingsReq struct {
	Name *string `json:"name"`
}

// handleUpdateTenantSettings updates owner-controlled tenant settings.
// PATCH /v1/owner/settings
func (s *Server) handleUpdateTenantSettings(c *gin.Context) {
	_, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}

	var req updateTenantSettingsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Name == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no settings provided"})
		return
	}

	var tenant models.Tenant
	if err := s.postgresDB.GetDB().First(&tenant, tenantID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch tenant"})
		return
	}

	trimmed := strings.TrimSpace(*req.Name)
	if trimmed == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name cannot be empty"})
		return
	}
	if len(trimmed) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name must be 100 characters or fewer"})
		return
	}
	oldName := tenant.Name
	if err := s.postgresDB.GetDB().Model(&tenant).Update("name", trimmed).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update name"})
		return
	}
	tenant.Name = trimmed

	s.recordAuditEvent(c, models.AuditSettingsUpdated, "tenant", fmt.Sprintf("%d", tenantID), AuditOpts{
		Category: models.AuditCategoryOwner,
		BeforeState: map[string]interface{}{
			"name": oldName,
		},
		AfterState: map[string]interface{}{
			"name": trimmed,
		},
	})

	c.JSON(http.StatusOK, tenantSettingsResponse{
		TenantID:   tenant.ID,
		Name:       tenant.Name,
		Plan:       tenant.Plan,
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
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}

	targetID := c.Param("user_id")
	if targetID == caller.ID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot promote yourself"})
		return
	}

	db := s.postgresDB.GetDB()

	var targetMembership models.TenantMembership
	if err := db.Where("tenant_id = ? AND user_id = ?", tenantID, targetID).First(&targetMembership).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	if targetMembership.OrgRole == models.RoleOwner {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user is already the owner"})
		return
	}
	if targetMembership.OrgRole == models.RoleAdmin {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user is already an admin"})
		return
	}

	if err := db.Model(&models.TenantMembership{}).
		Where("tenant_id = ? AND user_id = ?", tenantID, targetID).
		Update("org_role", models.RoleAdmin).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to promote user"})
		return
	}

	s.recordAuditEvent(c, models.AuditMemberPromoted, "membership", targetID, AuditOpts{
		Category: models.AuditCategoryTeam,
		BeforeState: map[string]interface{}{
			"role": targetMembership.OrgRole,
		},
		AfterState: map[string]interface{}{
			"role": models.RoleAdmin,
		},
	})

	var target models.User
	db.Where("id = ?", targetID).First(&target)
	slog.Info("user_promoted_to_admin", "email", target.Email, "by", caller.Email)
	c.JSON(http.StatusOK, gin.H{"message": "User promoted to admin successfully", "user": toUserResponse(target, models.RoleAdmin, targetMembership.Status)})
}

// handleDemoteAdmin demotes an admin to editor (owner only).
// DELETE /v1/owner/users/:user_id/demote-admin
func (s *Server) handleDemoteAdmin(c *gin.Context) {
	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}

	targetID := c.Param("user_id")
	db := s.postgresDB.GetDB()

	var targetMembership models.TenantMembership
	if err := db.Where("tenant_id = ? AND user_id = ?", tenantID, targetID).First(&targetMembership).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	if targetMembership.OrgRole != models.RoleAdmin {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user is not an admin"})
		return
	}

	if err := db.Model(&models.TenantMembership{}).
		Where("tenant_id = ? AND user_id = ?", tenantID, targetID).
		Update("org_role", models.RoleEditor).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to demote admin"})
		return
	}

	s.recordAuditEvent(c, models.AuditMemberDemoted, "membership", targetID, AuditOpts{
		Category: models.AuditCategoryTeam,
		BeforeState: map[string]interface{}{
			"role": models.RoleAdmin,
		},
		AfterState: map[string]interface{}{
			"role": models.RoleEditor,
		},
	})

	var target models.User
	db.Where("id = ?", targetID).First(&target)
	slog.Info("admin_demoted_to_editor", "email", target.Email, "by", caller.Email)
	c.JSON(http.StatusOK, gin.H{"message": "Admin demoted to editor successfully", "user": toUserResponse(target, models.RoleEditor, targetMembership.Status)})
}

// handleTransferOwnership transfers ownership to another active tenant member.
// POST /v1/owner/transfer-ownership
func (s *Server) handleTransferOwnership(c *gin.Context) {
	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
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

	db := s.postgresDB.GetDB()

	// Look up the new owner's membership in this tenant.
	var newOwnerMembership models.TenantMembership
	if err := db.Where("tenant_id = ? AND user_id = ?", tenantID, req.NewOwnerID).First(&newOwnerMembership).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "target user not found in your tenant"})
		return
	}

	if newOwnerMembership.Status != models.StatusActive {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot transfer ownership to a suspended user"})
		return
	}

	var newOwner models.User
	db.Where("id = ?", req.NewOwnerID).First(&newOwner)

	tx := db.Begin()
	// Demote current owner to admin.
	if err := tx.Model(&models.TenantMembership{}).
		Where("tenant_id = ? AND user_id = ?", tenantID, caller.ID).
		Update("org_role", models.RoleAdmin).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to transfer ownership"})
		return
	}
	// Promote new owner.
	if err := tx.Model(&models.TenantMembership{}).
		Where("tenant_id = ? AND user_id = ?", tenantID, req.NewOwnerID).
		Update("org_role", models.RoleOwner).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to transfer ownership"})
		return
	}
	tx.Commit()

	s.recordAuditEvent(c, models.AuditOwnershipTransferred, "tenant", fmt.Sprintf("%d", tenantID), AuditOpts{
		Category: models.AuditCategoryOwner,
		Metadata: map[string]interface{}{
			"from": caller.Email,
			"to":   newOwner.Email,
		},
	})

	slog.Info("ownership_transferred", "from", caller.Email, "to", newOwner.Email, "tenant_id", tenantID)
	c.JSON(http.StatusOK, gin.H{
		"message":   "Ownership transferred successfully",
		"new_owner": newOwner.Email,
	})
}

// ── Plan change ───────────────────────────────────────────────────────────────

type changePlanReq struct {
	Plan string `json:"plan" binding:"required"`
}

// applyPlanChange enforces downgrade limits and updates tenant.plan.
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

	// Provider key count
	if newLimits.MaxProviderKeys != -1 {
		var activeProviderKeys int64
		db.Model(&models.ProviderKey{}).
			Where("tenant_id = ? AND revoked = false", tenantID).
			Count(&activeProviderKeys)
		if int(activeProviderKeys) > newLimits.MaxProviderKeys {
			return http.StatusUnprocessableEntity, gin.H{
				"error":              "downgrade_blocked",
				"reason":             "provider_keys_exceed_limit",
				"message":            fmt.Sprintf("The %s plan allows %d provider key(s), but this tenant has %d active key(s). Revoke %d key(s) before downgrading.", newPlan, newLimits.MaxProviderKeys, activeProviderKeys, int(activeProviderKeys)-newLimits.MaxProviderKeys),
				"limit":              newLimits.MaxProviderKeys,
				"provider_key_count": activeProviderKeys,
			}
		}
	}

	// Member count from tenant_memberships (all statuses, matching invite enforcement)
	if newLimits.MaxMembers != -1 {
		var memberCount int64
		db.Model(&models.TenantMembership{}).Where("tenant_id = ?", tenantID).Count(&memberCount)
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

	// Project count
	if newLimits.MaxProjects != -1 {
		var projectCount int64
		db.Model(&models.Project{}).Where("tenant_id = ? AND status = ?", tenantID, models.ProjectStatusActive).Count(&projectCount)
		if int(projectCount) > newLimits.MaxProjects {
			return http.StatusUnprocessableEntity, gin.H{
				"error":         "downgrade_blocked",
				"reason":        "projects_exceed_limit",
				"message":       fmt.Sprintf("The %s plan allows %d project(s), but this tenant has %d active project(s). Archive %d project(s) before downgrading.", newPlan, newLimits.MaxProjects, projectCount, int(projectCount)-newLimits.MaxProjects),
				"limit":         newLimits.MaxProjects,
				"project_count": projectCount,
			}
		}
	}

	// Budget limit count
	if newLimits.MaxBudgetLimits != -1 {
		var budgetCount int64
		db.Model(&models.BudgetLimit{}).Where("tenant_id = ?", tenantID).Count(&budgetCount)
		if int(budgetCount) > newLimits.MaxBudgetLimits {
			return http.StatusUnprocessableEntity, gin.H{
				"error":        "downgrade_blocked",
				"reason":       "budget_limits_exceed_limit",
				"message":      fmt.Sprintf("The %s plan allows %d spend limit(s), but this tenant has %d. Remove %d spend limit(s) before downgrading.", newPlan, newLimits.MaxBudgetLimits, budgetCount, int(budgetCount)-newLimits.MaxBudgetLimits),
				"limit":        newLimits.MaxBudgetLimits,
				"budget_count": budgetCount,
			}
		}
	}

	// Rate limit count
	if newLimits.MaxRateLimits != -1 {
		var rlCount int64
		db.Model(&models.RateLimit{}).Where("tenant_id = ?", tenantID).Count(&rlCount)
		if int(rlCount) > newLimits.MaxRateLimits {
			return http.StatusUnprocessableEntity, gin.H{
				"error":            "downgrade_blocked",
				"reason":           "rate_limits_exceed_limit",
				"message":          fmt.Sprintf("The %s plan allows %d rate limit(s), but this tenant has %d. Remove %d rate limit(s) before downgrading.", newPlan, newLimits.MaxRateLimits, rlCount, int(rlCount)-newLimits.MaxRateLimits),
				"limit":            newLimits.MaxRateLimits,
				"rate_limit_count": rlCount,
			}
		}
	}

	// Notification channel count
	if newLimits.MaxNotificationChannels != -1 {
		var ncCount int64
		db.Model(&models.NotificationChannel{}).Where("tenant_id = ?", tenantID).Count(&ncCount)
		if int(ncCount) > newLimits.MaxNotificationChannels {
			return http.StatusUnprocessableEntity, gin.H{
				"error":         "downgrade_blocked",
				"reason":        "notification_channels_exceed_limit",
				"message":       fmt.Sprintf("The %s plan allows %d notification channel(s), but this tenant has %d. Remove %d channel(s) before downgrading.", newPlan, newLimits.MaxNotificationChannels, ncCount, int(ncCount)-newLimits.MaxNotificationChannels),
				"limit":         newLimits.MaxNotificationChannels,
				"channel_count": ncCount,
			}
		}
	}

	// ── Apply ────────────────────────────────────────────────────────────────
	if err := db.Model(&tenant).Update("plan", newPlan).Error; err != nil {
		return http.StatusInternalServerError, gin.H{"error": "failed to update plan"}
	}

	slog.Info("tenant_plan_changed", "tenant_id", tenantID, "from_plan", tenant.Plan, "to_plan", newPlan)
	return 0, nil
}

// handleChangePlan lets the tenant owner change their own plan.
// PATCH /v1/owner/plan
func (s *Server) handleChangePlan(c *gin.Context) {
	_, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}

	var req changePlanReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if status, body := s.applyPlanChange(tenantID, req.Plan); status != 0 {
		c.JSON(status, body)
		return
	}

	// Return updated settings in the same shape as GET /v1/owner/settings.
	var tenant models.Tenant
	s.postgresDB.GetDB().First(&tenant, tenantID)
	c.JSON(http.StatusOK, tenantSettingsResponse{
		TenantID:   tenant.ID,
		Name:       tenant.Name,
		Plan:       tenant.Plan,
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
		PlanLimits: models.GetPlanLimits(tenant.Plan),
	})
}
