package app

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// registerOAuthMetadata 暴露 RFC 9728 / RFC 8414 的元数据。
func registerOAuthMetadata(engine *gin.Engine) {
	engine.GET("/.well-known/oauth-authorization-server", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"issuer":                                "go-saas-lite",
			"authorization_endpoint":                "/api/tenant/v1/oauth/authorize",
			"token_endpoint":                        "/api/tenant/v1/oauth/token",
			"registration_endpoint":                 "/api/tenant/v1/oauth/register",
			"revocation_endpoint":                   "/api/tenant/v1/oauth/revoke",
			"response_types_supported":              []string{"code"},
			"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
			"code_challenge_methods_supported":      []string{"S256"},
			"token_endpoint_auth_methods_supported": []string{"none"},
			"scopes_supported":                      []string{"mcp"},
		})
	})

	engine.GET("/.well-known/oauth-protected-resource", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"resource":              "/api/tenant/v1/mcp",
			"authorization_servers": []string{"go-saas-lite"},
			"scopes_supported":      []string{"mcp"},
			"bearer_methods_supported": []string{
				"header",
			},
		})
	})
}
