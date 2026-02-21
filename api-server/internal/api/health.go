package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "tokengate-api",
	})
}

func (s *Server) handleNoRoute(c *gin.Context) {
	c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
}
