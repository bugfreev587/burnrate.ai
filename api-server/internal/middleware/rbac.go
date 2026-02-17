package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/xiaoboyu/burnrate-ai/api-server/internal/models"
)

const ContextKeyUser = "current_user"

type RBACMiddleware struct {
	db *gorm.DB
}

func NewRBACMiddleware(db *gorm.DB) *RBACMiddleware {
	return &RBACMiddleware{db: db}
}

// RequireUser loads the user identified by X-User-ID header and puts it in context.
func (r *RBACMiddleware) RequireUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetHeader("X-User-ID")
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing X-User-ID header"})
			c.Abort()
			return
		}
		var user models.User
		if err := r.db.First(&user, "id = ?", userID).Error; err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
			c.Abort()
			return
		}
		if !user.IsActive() {
			c.JSON(http.StatusForbidden, gin.H{"error": "user_suspended"})
			c.Abort()
			return
		}
		c.Set(ContextKeyUser, &user)
		c.Next()
	}
}

func (r *RBACMiddleware) RequireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		u, ok := GetUserFromContext(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}
		if !u.HasPermission(role) {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient_permissions"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func (r *RBACMiddleware) RequireViewer() gin.HandlerFunc { return r.RequireRole(models.RoleViewer) }
func (r *RBACMiddleware) RequireEditor() gin.HandlerFunc { return r.RequireRole(models.RoleEditor) }
func (r *RBACMiddleware) RequireAdmin() gin.HandlerFunc  { return r.RequireRole(models.RoleAdmin) }
func (r *RBACMiddleware) RequireOwner() gin.HandlerFunc  { return r.RequireRole(models.RoleOwner) }

// GetUserFromContext retrieves the authenticated user from gin context.
func GetUserFromContext(c *gin.Context) (*models.User, bool) {
	v, exists := c.Get(ContextKeyUser)
	if !exists {
		return nil, false
	}
	u, ok := v.(*models.User)
	return u, ok
}
