package stats

import (
	"go.uber.org/fx"

	"github.com/cuiwenguang/go-saas-lite/internal/routes"
)

// Module 装配租户总览统计模块。
func Module() fx.Option {
	return fx.Module("tenant.stats",
		fx.Provide(NewService, NewHandler),
		fx.Invoke(registerRoutes),
	)
}

func registerRoutes(r routes.Routes, h *Handler) {
	h.Register(r.Tenant)
}
