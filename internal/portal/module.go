package portal

import (
	"go.uber.org/fx"

	"github.com/cuiwenguang/go-saas-lite/internal/routes"
)

// Module 装配营销门户与自助注册。
func Module(opts Options) fx.Option {
	return fx.Module("portal",
		fx.Supply(opts),
		fx.Provide(NewService, NewHandler),
		fx.Invoke(registerRoutes),
	)
}

func registerRoutes(r routes.Routes, h *Handler) {
	if r.Site != nil {
		h.Register(r.Site)
	}
}
