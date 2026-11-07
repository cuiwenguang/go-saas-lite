package conf

import (
	"context"

	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"

	"github.com/cuiwenguang/go-saas-lite/internal/routes"
	"github.com/cuiwenguang/go-saas-lite/internal/security/authz"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// Module 装配租户配置模块。
func Module() fx.Option {
	return fx.Module("tenant.conf",
		fx.Provide(newService, NewHandler),
		fx.Invoke(registerRoutes),
		fx.Invoke(func() error { return authz.Register(PermRead, PermUpdate) }),
	)
}

// newService 构造带本地缓存与失效广播的配置服务。
func newService(lc fx.Lifecycle, db tenancy.DB, client *redis.Client) *Service {
	store := NewStore(db).WithRedis(client)
	if client != nil {
		ctx, cancel := context.WithCancel(context.Background())
		lc.Append(fx.Hook{
			OnStart: func(context.Context) error {
				go store.cache.subscribe(ctx)
				return nil
			},
			OnStop: func(context.Context) error {
				cancel()
				return nil
			},
		})
	}

	return &Service{store: store}
}

func registerRoutes(r routes.Routes, h *Handler) {
	h.Register(r.Tenant)
}
