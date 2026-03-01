package middleware

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

const (
	ContextKeyUser          = "user"
	ContextKeyTenantID      = "tenant_id"
	ContextKeyOrgRole       = "org_role"
	ErrCodeUnauthorized     = "unauthorized"
	ErrCodeForbidden        = "forbidden"
	ErrCodeUserSuspended    = "user_suspended"
	ErrCodeInsufficientRole = "insufficient_role"
)

type RBACMiddleware struct {
	db *gorm.DB
}

func NewRBACMiddleware(db *gorm.DB) *RBACMiddleware {
	return &RBACMiddleware{db: db}
}

// RequireUser loads the user from X-User-ID header, reads X-Tenant-Id,
// loads the TenantMembership, and stores user + tenant_id + org_role in context.
func (m *RBACMiddleware) RequireUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetHeader("X-User-ID")
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   ErrCodeUnauthorized,
				"message": "Authentication required. Please sign in.",
			})
			c.Abort()
			return
		}

		var user models.User
		if err := m.db.Where("id = ?", userID).First(&user).Error; err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   ErrCodeUnauthorized,
				"message": "User not found. Please sign in again.",
			})
			c.Abort()
			return
		}

		if !user.IsActive() {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   ErrCodeUserSuspended,
				"message": "Your account has been suspended. Please contact your organization administrator.",
			})
			c.Abort()
			return
		}

		// Read tenant from X-Tenant-Id header.
		tenantIDStr := c.GetHeader("X-Tenant-Id")
		if tenantIDStr == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "missing_tenant_id",
				"message": "X-Tenant-Id header is required.",
			})
			c.Abort()
			return
		}

		tenantID64, err := strconv.ParseUint(tenantIDStr, 10, 64)
		if err != nil || tenantID64 == 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_tenant_id",
				"message": "X-Tenant-Id must be a valid positive integer.",
			})
			c.Abort()
			return
		}
		tenantID := uint(tenantID64)

		// Load TenantMembership.
		var membership models.TenantMembership
		if err := m.db.Where("tenant_id = ? AND user_id = ?", tenantID, userID).First(&membership).Error; err != nil {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   ErrCodeForbidden,
				"message": "You are not a member of this tenant.",
			})
			c.Abort()
			return
		}

		if membership.Status != models.StatusActive {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   ErrCodeUserSuspended,
				"message": "Your membership in this tenant has been suspended.",
			})
			c.Abort()
			return
		}

		c.Set(ContextKeyUser, &user)
		c.Set(ContextKeyTenantID, tenantID)
		c.Set(ContextKeyOrgRole, membership.OrgRole)
		c.Next()
	}
}

// RequireOrgRole is a convenience middleware that checks the org role meets a minimum level.
// Used as a simple gate during transition.
func (m *RBACMiddleware) RequireOrgRole(minRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgRole := GetOrgRoleFromContext(c)
		if models.RoleLevel(orgRole) < models.RoleLevel(minRole) {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   ErrCodeInsufficientRole,
				"message": fmt.Sprintf("You don't have permission to perform this action. Required role: %s", minRole),
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

// GetUserFromContext retrieves the authenticated user from gin context.
func GetUserFromContext(c *gin.Context) (*models.User, bool) {
	v, exists := c.Get(ContextKeyUser)
	if !exists {
		return nil, false
	}
	u, ok := v.(*models.User)
	return u, ok
}

// GetTenantIDFromContext retrieves tenant ID from context.
func GetTenantIDFromContext(c *gin.Context) (uint, bool) {
	if tid, exists := c.Get(ContextKeyTenantID); exists {
		return tid.(uint), true
	}
	if akI, exists := c.Get(ContextKeyAPIKey); exists {
		ak := akI.(*models.APIKey)
		return ak.TenantID, true
	}
	return 0, false
}

// GetOrgRoleFromContext retrieves the org role from context.
func GetOrgRoleFromContext(c *gin.Context) string {
	if role, exists := c.Get(ContextKeyOrgRole); exists {
		return role.(string)
	}
	return ""
}
