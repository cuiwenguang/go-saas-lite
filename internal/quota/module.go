package quota

import (
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"

	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// Module 装配配额模块。
func Module() fx.Option {
	return fx.Module("quota",
		fx.Provide(newService),
	)
}

// newService 未启用 Redis 时返回 nil，配额与限流自动降级。
func newService(platform tenancy.PlatformDB, client *redis.Client) *Service {
	if client == nil {
		return nil
	}
	return NewService(platform, client)
}
