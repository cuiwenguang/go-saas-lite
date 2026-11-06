package app

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/fx"

	"github.com/cuiwenguang/go-saas-lite/internal/mcp"
	"github.com/cuiwenguang/go-saas-lite/internal/platform"
	"github.com/cuiwenguang/go-saas-lite/internal/quota"
	"github.com/cuiwenguang/go-saas-lite/internal/security/apikey"
	"github.com/cuiwenguang/go-saas-lite/internal/security/auth"
	"github.com/cuiwenguang/go-saas-lite/internal/security/authz"
	"github.com/cuiwenguang/go-saas-lite/internal/security/oauth"
	"github.com/cuiwenguang/go-saas-lite/internal/tenant"
	"github.com/cuiwenguang/go-saas-lite/internal/worker"
	"github.com/cuiwenguang/go-saas-lite/pkg/obs"
)

// New 组装 API 进程的依赖图。
//
// 约束（readme 3.3）：
//   - 业务包只导出普通构造函数，不做全局单例；
//   - fx.Invoke 只做注册与触发，不写业务逻辑；
//   - CI 用 fx.ValidateApp 校验依赖图。
func New(cfg Config) fx.Option {
	return fx.Options(
		Core(cfg),

		authz.Module(),
		apikey.Module(),
		quota.Module(),
		mcp.Module(),
		oauth.Module(),
		platform.Module(platform.Options{SecureCookies: cfg.App.Env != "dev"}),
		tenant.Module(),

		// dev 下把后台 worker 一并挂到 api 进程，免去同时起多个服务。
		optionalDevWorker(cfg),

		fx.Provide(
			newIssuer,
			obs.NewMetrics,
			obs.NewHealth,
			newTracerProvider,
			fx.Annotate(NewTenantLookup, fx.As(new(TenantLookup))),
			NewRouter,
			NewServer,
		),

		fx.Invoke(registerHealth),

		// 构造顺序决定 OnStop 的执行顺序（fx 逆序执行 Hook）：
		//   1. tracer provider 先构造 → 其 OnStop 最后执行（server 停完再 flush）；
		//   2. http.Server 后构造 → 其 OnStop 最先执行（先停接流量）。
		fx.Invoke(func(*trace.TracerProvider) {}),
		fx.Invoke(func(*http.Server) {}),
	)
}

// optionalDevWorker 在 dev 环境下把后台 worker 一并挂到 api 进程，方便单进程调试。
// 生产环境通过独立的 cmd/worker 进程运行。
func optionalDevWorker(cfg Config) fx.Option {
	if cfg.App.Env != "dev" {
		return fx.Options()
	}

	return fx.Options(
		fx.Supply(worker.Options{
			Interval:           time.Duration(cfg.Worker.IntervalSeconds) * time.Second,
			CoolingDays:        cfg.Worker.CoolingDays,
			AuditRetentionDays: cfg.Audit.RetentionDays,
		}),
		fx.Provide(worker.New),
		fx.Invoke(func(*worker.Worker) {}),
	)
}

// Core 提供日志、双连接池与 Redis，api 与 worker 共用。
func Core(cfg Config) fx.Option {
	return fx.Options(
		fx.Supply(cfg),
		fx.Provide(
			newLogger,
			newPlatformGorm,
			newTenantGorm,
			newTenantDB,
			newPlatformDB,
			newRedisClient,
		),
	)
}

func newLogger(cfg Config) *slog.Logger {
	return obs.NewLogger(cfg.Log.Level, cfg.Log.Format)
}

func newIssuer(cfg Config) *auth.Issuer {
	return auth.NewIssuer(
		cfg.Auth.Secret,
		cfg.Auth.IssuerName,
		auth.WithAccessTTL(time.Duration(cfg.Auth.AccessTTLSeconds)*time.Second),
	)
}

func newTracerProvider(lc fx.Lifecycle, cfg Config) (*trace.TracerProvider, error) {
	provider, err := obs.NewTracerProvider(cfg.App.Name, cfg.App.Env, cfg.Trace.Exporter)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			// 先 flush 再退出，避免丢失最后一次导出。
			if shutdownErr := provider.Shutdown(ctx); shutdownErr != nil {
				return shutdownErr
			}
			return nil
		},
	})

	return provider, nil
}
