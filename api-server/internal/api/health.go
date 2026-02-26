package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Server) handleHealth(c *gin.Context) {
	// Deep health check: verify DB connectivity.
	sqlDB, err := s.postgresDB.GetDB().DB()
	if err != nil || sqlDB.Ping() != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":  "degraded",
			"service": "tokengate-api",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "tokengate-api",
	})
}

func (s *Server) handleNoRoute(c *gin.Context) {
	c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
}
