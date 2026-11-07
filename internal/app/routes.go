package app

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	"github.com/cuiwenguang/go-saas-lite/internal/quota"
	"github.com/cuiwenguang/go-saas-lite/internal/routes"
	"github.com/cuiwenguang/go-saas-lite/internal/security/apikey"
	"github.com/cuiwenguang/go-saas-lite/internal/security/auth"
	"github.com/cuiwenguang/go-saas-lite/pkg/obs"
	"github.com/cuiwenguang/go-saas-lite/pkg/response"
)

// NewRouter 装配 HTTP 路由与中间件链。
//
// 顺序（readme 3.5，M3 完成通用段 + 两条鉴权链）：
// cors → recover → request_id → trace/metrics → logger → auth(guard) → handler
// 租户解析、权限、配额在后续里程碑插入到 auth 之后。
func NewRouter(cfg Config, log *slog.Logger, metrics *obs.Metrics, health *obs.Health, issuer *auth.Issuer, keys *apikey.Service, lookup TenantLookup, quotaService *quota.Service) (*gin.Engine, routes.Routes) {
	if cfg.App.Env != "dev" {
		gin.SetMode(gin.ReleaseMode)
	}

	engine := gin.New()
	engine.Use(cors(cfg.Server.AllowedOrigins))
	engine.Use(securityHeaders())
	engine.Use(obs.Recover(log, metrics))
	engine.Use(bodyLimit(cfg.Server.MaxBodyMB))
	engine.Use(response.RequestID())
	engine.Use(response.ClientIP())
	if cfg.Trace.Enabled {
		engine.Use(otelgin.Middleware(cfg.App.Name))
		engine.Use(obs.TraceContext())
	}
	if cfg.Metrics.Enabled {
		engine.Use(metrics.Middleware())
		engine.GET("/metrics", gin.WrapH(metrics.Handler()))
	}
	engine.Use(requestLogger(log))

	engine.GET("/health", health.Live)
	engine.GET("/readyz", health.Ready)

	// 免鉴权入口：OAuth 注册 / 授权需要租户上下文，先解析租户。
	public := engine.Group("/api/tenant/v1")
	public.Use(TenantResolver(lookup, cfg.Tenancy.BaseDomain))

	// 全局登录入口：邮箱全局唯一，登录时才知道租户，不能要求先把租户解析出来。
	tenantAuth := engine.Group("/api/tenant/v1")

	tenantChain := TenantChain(engine.Group("/api/tenant/v1"), issuer, lookup)
	if quotaService != nil {
		tenantChain.Use(quota.Middleware(quotaService))
	}

	// 三条入口链分开注册，平台 token、租户 token、Agent token 互不通用。
	routeGroups := routes.Routes{
		Site:           engine.Group(""),
		Public:         public,
		TenantAuth:     tenantAuth,
		PlatformPublic: engine.Group("/api/platform/v1"),
		Tenant:         tenantChain,
		Platform:       PlatformChain(engine.Group("/api/platform/v1"), issuer),
		MCP:            MCPChain(engine.Group("/api/tenant/v1/mcp"), issuer, keys, lookup),
	}

	registerOAuthMetadata(engine)
	registerDocs(engine, cfg.Docs.Enabled)

	return engine, routeGroups
}

// securityHeaders 设置基础安全响应头。
func securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		c.Next()
	}
}

// bodyLimit 限制普通请求体大小；文件上传与 MCP 使用各自的上限。
func bodyLimit(maxMB int) gin.HandlerFunc {
	if maxMB <= 0 {
		return func(c *gin.Context) { c.Next() }
	}

	limit := int64(maxMB) << 20
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead {
			c.Next()
			return
		}
		if strings.HasPrefix(c.Request.URL.Path, "/api/tenant/v1/files") {
			c.Next()
			return
		}

		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		c.Next()
	}
}

func requestLogger(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}

		log.InfoContext(c.Request.Context(), "http request",
			"method", c.Request.Method,
			"route", route,
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
		)
	}
}

// cors 只放行显式配置的来源，禁止通配；预检直接返回，不进后续链。
// 租户域名白名单在 M5 由 tenants.domain / subdomain 提供。
func cors(allowedOrigins []string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		if origin == "*" {
			continue
		}
		allowed[origin] = struct{}{}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			if _, ok := allowed[origin]; ok {
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Vary", "Origin")
				c.Header("Access-Control-Allow-Credentials", "true")
				c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
				c.Header("Access-Control-Allow-Headers", "Authorization,Content-Type,X-Tenant-Id,X-Request-Id,Idempotency-Key,Mcp-Session-Id")
			}
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
