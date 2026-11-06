package account

import (
	"go.uber.org/fx"

	"github.com/cuiwenguang/go-saas-lite/internal/routes"
)

// Module 装配平台会话模块；opts 由聚合层传入（cookie 安全开关）。
func Module(opts Options) fx.Option {
	return fx.Module("platform.account",
		fx.Provide(
			func() Options { return opts },
			NewService,
			NewHandler,
		),
		fx.Invoke(registerRoutes),
	)
}

func registerRoutes(r routes.Routes, h *Handler) {
	h.RegisterPublic(r.PlatformPublic)
	h.Register(r.Platform)
}
