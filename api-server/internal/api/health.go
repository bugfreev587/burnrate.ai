package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/xiaoboyu/tokengate/api-server/internal/middleware"
)

func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "tokengate-api",
	})
}

func (s *Server) handleTenantHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"tenant": c.GetString(middleware.ContextKeyTenantSlug),
	})
}

func (s *Server) handleNoRoute(c *gin.Context) {
	if strings.HasPrefix(c.Request.URL.Path, "/v1/") {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{
			"type":    middleware.ErrCodeTenantMissing,
			"message": "Tenant is required. Use /{tenant_slug}/v1/...",
		}})
		return
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
}
