package account

import (
	"go.uber.org/fx"

	"github.com/cuiwenguang/go-saas-lite/internal/routes"
)

// Module 装配租户用户登录与 API Key 模块。
func Module() fx.Option {
	return fx.Module("tenant.account",
		fx.Provide(NewService, NewHandler),
		fx.Invoke(registerRoutes),
	)
}

func registerRoutes(r routes.Routes, h *Handler) {
	h.RegisterPublic(r.Public)
	h.Register(r.Tenant)
}
