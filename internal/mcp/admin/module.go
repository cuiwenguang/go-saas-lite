// Package admin 提供 MCP 上游服务、工具的配置管理与调用日志查询。
package admin

import (
	"go.uber.org/fx"

	"github.com/cuiwenguang/go-saas-lite/internal/routes"
)

// Module 装配 MCP 管理接口。
func Module() fx.Option {
	return fx.Module("mcp.admin",
		fx.Provide(NewService, NewHandler),
		fx.Invoke(registerRoutes),
	)
}

func registerRoutes(r routes.Routes, h *Handler) {
	h.Register(r.Tenant)
}
