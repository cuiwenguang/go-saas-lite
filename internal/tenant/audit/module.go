package audit

import (
	"go.uber.org/fx"

	"github.com/cuiwenguang/go-saas-lite/internal/routes"
	"github.com/cuiwenguang/go-saas-lite/internal/security/authz"
)

// Module 装配租户审计模块。
func Module() fx.Option {
	return fx.Module("tenant.audit",
		fx.Provide(NewService, NewHandler),
		fx.Invoke(registerRoutes),
		fx.Invoke(func() error { return authz.Register(PermRead) }),
	)
}

func registerRoutes(r routes.Routes, h *Handler) {
	h.Register(r.Tenant)
}
