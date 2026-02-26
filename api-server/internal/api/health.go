package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func (s *Server) handleHealth(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	// Check Postgres connectivity.
	pgOk := true
	sqlDB, err := s.postgresDB.GetDB().DB()
	if err != nil || sqlDB.PingContext(ctx) != nil {
		pgOk = false
	}

	// Check Redis connectivity.
	redisOk := true
	if s.rdb != nil {
		if err := s.rdb.Ping(ctx).Err(); err != nil {
			redisOk = false
		}
	}

	status := "ok"
	httpStatus := http.StatusOK
	if !pgOk || !redisOk {
		status = "degraded"
		httpStatus = http.StatusServiceUnavailable
		slog.Warn("health_check_degraded", "postgres", pgOk, "redis", redisOk)
	}

	c.JSON(httpStatus, gin.H{
		"status":   status,
		"service":  "tokengate-api",
		"postgres": pgOk,
		"redis":    redisOk,
	})
}

func (s *Server) handleNoRoute(c *gin.Context) {
	c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
}
