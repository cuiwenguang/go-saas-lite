// Package routes 定义路由链分组类型，由 internal/app 构造、各业务模块消费。
package routes

import "github.com/gin-gonic/gin"

// Routes 暴露已装配好的路由链，模块通过 fx.Invoke 把 handler 挂上去。
type Routes struct {
	Site           *gin.RouterGroup // 根路径下的无鉴权入口（门户、签名下载）
	Public         *gin.RouterGroup // 租户域名下的免鉴权入口（OAuth 注册 / 授权），需租户上下文
	TenantAuth     *gin.RouterGroup // 全局登录入口（登录 / 刷新 / 登出），不需要租户上下文
	PlatformPublic *gin.RouterGroup // 平台登录
	Tenant         *gin.RouterGroup
	Platform       *gin.RouterGroup
	MCP            *gin.RouterGroup
}
