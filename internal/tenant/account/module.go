package account

import (
	"go.uber.org/fx"

	"github.com/cuiwenguang/go-saas-lite/internal/routes"
)

// Module 装配租户会话、资料与 API Key 模块。
func Module(opts Options) fx.Option {
	return fx.Module("tenant.account",
		fx.Provide(
			func() Options { return opts },
			NewService,
			NewHandler,
		),
		fx.Invoke(registerRoutes),
	)
}

func registerRoutes(r routes.Routes, h *Handler) {
	h.RegisterPublic(r.TenantAuth)
	h.Register(r.Tenant)
}
