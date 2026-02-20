package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

const (
	ContextKeyUser          = "user"
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

// RequireUser loads the user from X-User-ID header, checks active status,
// and stores the user + tenant_id in context.
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

		c.Set(ContextKeyUser, &user)
		c.Set("tenant_id", user.TenantID)
		c.Next()
	}
}

// RequireRole checks that the authenticated user has at least the required role level.
func (m *RBACMiddleware) RequireRole(requiredRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := GetUserFromContext(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   ErrCodeUnauthorized,
				"message": "Authentication required.",
			})
			c.Abort()
			return
		}
		if !user.HasPermission(requiredRole) {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   ErrCodeInsufficientRole,
				"message": "You don't have permission to perform this action. Required role: " + requiredRole,
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

func (m *RBACMiddleware) RequireOwner() gin.HandlerFunc  { return m.RequireRole(models.RoleOwner) }
func (m *RBACMiddleware) RequireAdmin() gin.HandlerFunc  { return m.RequireRole(models.RoleAdmin) }
func (m *RBACMiddleware) RequireEditor() gin.HandlerFunc { return m.RequireRole(models.RoleEditor) }
func (m *RBACMiddleware) RequireViewer() gin.HandlerFunc { return m.RequireRole(models.RoleViewer) }

// GetUserFromContext retrieves the authenticated user from gin context.
func GetUserFromContext(c *gin.Context) (*models.User, bool) {
	v, exists := c.Get(ContextKeyUser)
	if !exists {
		return nil, false
	}
	u, ok := v.(*models.User)
	return u, ok
}

// GetTenantIDFromContext retrieves tenant ID from user or API key context.
func GetTenantIDFromContext(c *gin.Context) (uint, bool) {
	if user, ok := GetUserFromContext(c); ok {
		return user.TenantID, true
	}
	if akI, exists := c.Get(ContextKeyAPIKey); exists {
		ak := akI.(*models.APIKey)
		return ak.TenantID, true
	}
	if tid, exists := c.Get("tenant_id"); exists {
		return tid.(uint), true
	}
	return 0, false
}
