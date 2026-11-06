package authz

import (
	"context"

	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"

	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// Module 装配权限求值模块，并在启动时登记内置角色权限点。
func Module() fx.Option {
	return fx.Module("authz",
		fx.Provide(fx.Annotate(newService, fx.As(new(Authorizer)))),
		fx.Invoke(func() error { return RegisterBuiltins() }),
	)
}

// newService 组装权限求值服务，并订阅跨副本失效广播。
func newService(lc fx.Lifecycle, db tenancy.DB, client *redis.Client) *Service {
	service := NewService(NewGormSource(db))
	if client != nil {
		service.WithBroadcaster(NewRedisBroadcaster(client))
	}

	ctx, cancel := context.WithCancel(context.Background())
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go func() { _ = service.WatchInvalidation(ctx) }()
			return nil
		},
		OnStop: func(context.Context) error {
			cancel()
			return nil
		},
	})

	return service
}
