package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// handleCreateProviderKey stores an upstream LLM provider API key.
// POST /v1/admin/provider_keys
func (s *Server) handleCreateProviderKey(c *gin.Context) {
	// TODO: implement
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}

// handleListProviderKeys lists stored provider keys (masked).
// GET /v1/admin/provider_keys
func (s *Server) handleListProviderKeys(c *gin.Context) {
	// TODO: implement
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}

// handleRevokeProviderKey revokes a provider key.
// DELETE /v1/admin/provider_keys/:key_id
func (s *Server) handleRevokeProviderKey(c *gin.Context) {
	// TODO: implement
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}
