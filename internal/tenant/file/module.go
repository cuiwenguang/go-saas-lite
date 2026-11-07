package file

import (
	"go.uber.org/fx"

	"github.com/cuiwenguang/go-saas-lite/internal/routes"
	"github.com/cuiwenguang/go-saas-lite/internal/worker"
)

// Module 装配租户文件模块。
func Module(opts Options) fx.Option {
	return fx.Module("tenant.file",
		fx.Supply(opts),
		fx.Provide(NewService, NewHandler),
		fx.Provide(fx.Annotate(
			func(s *Service) worker.Cleaner { return s },
			fx.As(new(worker.Cleaner)),
			fx.ResultTags(`group:"tenant_cleaners"`),
		)),
		fx.Invoke(registerRoutes),
	)
}

func registerRoutes(r routes.Routes, h *Handler) {
	h.Register(r.Tenant, r.Site)
}
