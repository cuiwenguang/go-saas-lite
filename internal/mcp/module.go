package mcp

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"

	"github.com/cuiwenguang/go-saas-lite/internal/quota"
	"github.com/cuiwenguang/go-saas-lite/internal/routes"
	"github.com/cuiwenguang/go-saas-lite/internal/security/authz"
)

// Module 装配 MCP 端点：工具网关 + 调用日志 + 协议处理器 + 路由。
func Module() fx.Option {
	return fx.Module("mcp",
		fx.Provide(NewGateway, NewCallLog, newHandler),
		fx.Invoke(registerRoutes),
	)
}

// newHandler 组装 MCP 端点；Redis 不可用时退化为内存会话。
func newHandler(
	gateway *Gateway,
	authorizer authz.Authorizer,
	client *redis.Client,
	calls *CallLog,
	quotaService *quota.Service,
) *Handler {
	var sessions SessionStore = NewMemorySessions()
	if client != nil {
		sessions = NewRedisSessions(client, 30*time.Minute)
	}

	return NewHandler(Options{
		Provider:   gateway,
		Authorizer: authorizer,
		Quota:      quotaService,
		Sessions:   sessions,
		Calls:      calls,
	})
}

func registerRoutes(r routes.Routes, h *Handler) {
	r.MCP.POST("", gin.WrapH(h))
}
