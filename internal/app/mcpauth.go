package app

import (
	"log/slog"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/cuiwenguang/go-saas-lite/internal/security/apikey"
	"github.com/cuiwenguang/go-saas-lite/internal/security/auth"
	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/obs"
	"github.com/cuiwenguang/go-saas-lite/pkg/response"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// MCPAuth 是 MCP 入口的鉴权：接受用户级 API Key 与 aud=mcp 的 OAuth token。
// Web 的 tenant-api token 不能通过（readme 1.11）。
func MCPAuth(issuer *auth.Issuer, keys *apikey.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := strings.TrimSpace(strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer "))

		var scope tenancy.Scope

		if strings.HasPrefix(raw, apikey.KeyPrefix) {
			identity, err := keys.Authenticate(c.Request.Context(), raw)
			if err != nil {
				response.Fail(c, err)
				return
			}
			scope = tenancy.Scope{
				TenantID: identity.TenantID,
				UserID:   identity.UserID,
				ClientID: identity.ClientID,
				Actor:    tenancy.ActorTenant,
			}
		} else {
			authenticated, ok := auth.Authenticate(c, issuer, auth.Rule{
				Audience: auth.AudienceMCP,
				Actor:    tenancy.ActorTenant,
			})
			if !ok {
				return
			}
			scope = authenticated
		}

		if scope.TenantID == "" || scope.UserID == "" {
			response.Fail(c, apperr.New(apperr.CodeUnauthenticated, "missing tenant or user identity"))
			return
		}

		ctx := tenancy.WithScope(c.Request.Context(), scope)
		ctx = obs.WithLogAttrs(ctx,
			slog.String("tenant_id", scope.TenantID),
			slog.String("user_id", scope.UserID),
			slog.String("client_id", scope.ClientID),
		)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}
