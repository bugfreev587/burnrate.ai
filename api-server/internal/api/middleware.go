package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// LoggerMiddleware logs requests that are slow (>1s) or returned an error (5xx).
func LoggerMiddleware() gin.HandlerFunc {
	return gin.LoggerWithConfig(gin.LoggerConfig{
		SkipPaths: []string{"/v1/health"},
		Formatter: func(param gin.LogFormatterParams) string {
			// Only log slow requests or server errors to keep log volume low.
			if param.Latency < time.Second && param.StatusCode < 500 {
				return ""
			}
			return fmt.Sprintf("[GIN] %v | %3d | %13v | %s %s\n",
				param.TimeStamp.Format("2006/01/02 15:04:05"),
				param.StatusCode,
				param.Latency,
				param.Method,
				param.Path,
			)
		},
	})
}

// CORSMiddleware sets permissive CORS headers for allowed origins.
func CORSMiddleware(allowedOrigins []string) gin.HandlerFunc {
	originSet := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		originSet[o] = true
	}
	allowAll := len(allowedOrigins) == 0 || (len(allowedOrigins) == 1 && allowedOrigins[0] == "*")

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if allowAll || originSet[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers",
			"Authorization, Content-Type, X-Api-Key, X-User-ID, "+
				"X-TokenGate-Key, X-TokenGate-Provider, X-TokenGate-User, "+
				"X-TokenGate-Project, X-TokenGate-Session")
		c.Header("Access-Control-Allow-Credentials", "true")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// RateLimitMiddleware applies a per-IP rate limiter (token bucket).
func RateLimitMiddleware(rpm int) gin.HandlerFunc {
	if rpm <= 0 {
		rpm = 120
	}
	type limiterEntry struct {
		limiter  *rate.Limiter
		lastSeen time.Time
	}
	var mu sync.Mutex
	limiters := make(map[string]*limiterEntry)

	// Cleanup goroutine
	go func() {
		for range time.Tick(time.Minute) {
			mu.Lock()
			for ip, e := range limiters {
				if time.Since(e.lastSeen) > 5*time.Minute {
					delete(limiters, ip)
				}
			}
			mu.Unlock()
		}
	}()

	r := rate.Every(time.Minute / time.Duration(rpm))

	return func(c *gin.Context) {
		ip := c.ClientIP()
		mu.Lock()
		e, ok := limiters[ip]
		if !ok {
			e = &limiterEntry{limiter: rate.NewLimiter(r, rpm)}
			limiters[ip] = e
		}
		e.lastSeen = time.Now()
		lim := e.limiter
		mu.Unlock()

		if !lim.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "rate_limit_exceeded"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// DebugHeadersMiddleware logs every incoming request's full header set.
// Enable by setting DEBUG_HEADERS=true in the environment.
// NEVER enable in production — token prefixes are logged.
func DebugHeadersMiddleware() gin.HandlerFunc {
	enabled := os.Getenv("DEBUG_HEADERS") == "true"
	if !enabled {
		return func(c *gin.Context) { c.Next() }
	}
	log.Println("[DEBUG_HEADERS] header debug logging is ENABLED")

	return func(c *gin.Context) {
		r := c.Request

		// ── All headers as pretty JSON ────────────────────────────────────────
		allHeaders := make(map[string][]string, len(r.Header))
		for k, v := range r.Header {
			allHeaders[k] = v
		}
		headersJSON, _ := json.MarshalIndent(allHeaders, "  ", "  ")

		// ── Authorization: safe summary only ──────────────────────────────────
		authSummary := "absent"
		if auth := r.Header.Get("Authorization"); auth != "" {
			preview := auth
			if len(preview) > 20 {
				preview = preview[:20] + "..."
			}
			authSummary = fmt.Sprintf("present | len=%d | prefix=%q", len(auth), preview)
		}

		// ── Proxy / edge headers ──────────────────────────────────────────────
		proxyHeaders := []string{
			"X-Forwarded-For", "X-Forwarded-Proto", "X-Forwarded-Host",
			"X-Real-IP", "Forwarded", "CF-Connecting-IP", "Fly-Client-IP",
		}
		var proxyLines strings.Builder
		for _, h := range proxyHeaders {
			v := r.Header.Get(h)
			if v == "" {
				v = "(absent)"
			}
			fmt.Fprintf(&proxyLines, "  %-22s %s\n", h+":", v)
		}

		// ── TokenGate custom headers ──────────────────────────────────────────
		var tgLines strings.Builder
		for k := range r.Header {
			if strings.HasPrefix(strings.ToLower(k), "x-tokengate-") {
				fmt.Fprintf(&tgLines, "  %-32s %s\n", k+":", r.Header.Get(k))
			}
		}
		if tgLines.Len() == 0 {
			tgLines.WriteString("  (none)\n")
		}

		log.Printf(`
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ INCOMING REQUEST ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  Method:     %s
  URL:        %s
  Host:       %s
  Proto:      %s
  RemoteAddr: %s

── Proxy / Edge Headers ──────────────────────────────────────────────────────
%s
── TokenGate Headers ─────────────────────────────────────────────────────────
%s
── Authorization ─────────────────────────────────────────────────────────────
  %s

── All Headers (JSON) ────────────────────────────────────────────────────────
  %s
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
`,
			r.Method,
			r.URL.String(),
			r.Host,
			r.Proto,
			r.RemoteAddr,
			proxyLines.String(),
			tgLines.String(),
			authSummary,
			string(headersJSON),
		)

		c.Next()
	}
}
