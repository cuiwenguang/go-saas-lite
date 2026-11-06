package grant

import (
	"go.uber.org/fx"

	"github.com/cuiwenguang/go-saas-lite/internal/routes"
)

// Module 装配 OAuth 授权管理模块。
func Module() fx.Option {
	return fx.Module("oauth.grant",
		fx.Provide(NewService, NewHandler),
		fx.Invoke(registerRoutes),
	)
}

func registerRoutes(r routes.Routes, h *Handler) {
	h.Register(r.Tenant)
}
