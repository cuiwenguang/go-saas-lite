package account

import (
	"go.uber.org/fx"

	"github.com/cuiwenguang/go-saas-lite/internal/routes"
)

// Module 装配平台登录模块。
func Module() fx.Option {
	return fx.Module("platform.account",
		fx.Provide(NewService, NewHandler),
		fx.Invoke(registerRoutes),
	)
}

func registerRoutes(r routes.Routes, h *Handler) {
	h.RegisterPublic(r.PlatformPublic)
}
