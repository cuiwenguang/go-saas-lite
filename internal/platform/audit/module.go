package audit

import (
	"go.uber.org/fx"

	"github.com/cuiwenguang/go-saas-lite/internal/routes"
)

// Module 装配平台审计模块。
func Module() fx.Option {
	return fx.Module("platform.audit",
		fx.Provide(NewService, NewHandler),
		fx.Invoke(registerRoutes),
	)
}

func registerRoutes(r routes.Routes, h *Handler) {
	h.Register(r.Platform)
}
