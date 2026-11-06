package oauth

import (
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"

	"github.com/cuiwenguang/go-saas-lite/internal/routes"
	"github.com/cuiwenguang/go-saas-lite/internal/security/auth"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// Module 装配 OAuth 端点模块。
func Module() fx.Option {
	return fx.Module("oauth",
		fx.Provide(newServer, NewHandler),
		fx.Invoke(registerRoutes),
	)
}

// newServer 未启用 Redis 时返回 nil，OAuth 路由不注册。
func newServer(platform tenancy.PlatformDB, tenantDB tenancy.DB, client *redis.Client, issuer *auth.Issuer) *Server {
	if client == nil {
		return nil
	}
	return NewServer(platform, tenantDB, client, issuer, Config{})
}

func registerRoutes(r routes.Routes, h *Handler) {
	if h == nil || h.server == nil {
		return
	}
	h.RegisterPublic(r.Public)
	h.Register(r.Tenant)
}
