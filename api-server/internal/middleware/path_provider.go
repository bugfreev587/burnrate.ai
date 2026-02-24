package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// pathProviderRule maps a URL path prefix to its allowed provider(s).
type pathProviderRule struct {
	Prefix    string
	Providers []string
}

// pathProviderRules defines which provider(s) are allowed for each proxy path.
// Order matters: first match wins, so more specific prefixes come first.
var pathProviderRules = []pathProviderRule{
	{"/v1/messages", []string{"anthropic"}},
	{"/v1/models", []string{"anthropic"}},
	{"/v1/responses", []string{"anthropic", "openai"}},
	{"/v1/openai/", []string{"openai"}},
	{"/v1/gemini/", []string{"gemini"}},
	{"/v1/bedrock/", []string{"bedrock"}},
	{"/v1/vertex/", []string{"vertex"}},
}

// PathProviderGuard rejects requests when the API key's provider does not match
// the provider implied by the request path. For example, an openai key hitting
// /v1/messages (Anthropic) returns 403.
//
// When no provider is set in context (e.g. gateway validation disabled), the
// guard is a no-op.
func PathProviderGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		keyProvider := c.GetString("provider")
		if keyProvider == "" {
			// No provider in context — gateway validation is disabled or key
			// wasn't resolved. Skip the check.
			c.Next()
			return
		}

		path := c.Request.URL.Path
		for _, rule := range pathProviderRules {
			if strings.HasPrefix(path, rule.Prefix) {
				if !containsStr(rule.Providers, keyProvider) {
					c.JSON(http.StatusForbidden, gin.H{"error": gin.H{
						"type":    "tg_provider_mismatch",
						"message": "API key provider \"" + keyProvider + "\" is not allowed for path " + path + ".",
					}})
					c.Abort()
					return
				}
				break
			}
		}
		c.Next()
	}
}

func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
