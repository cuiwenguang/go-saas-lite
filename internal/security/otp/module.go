package otp

import (
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"

	"github.com/cuiwenguang/go-saas-lite/pkg/sms"
)

// Module 装配验证码服务；未启用 Redis 或短信通道时返回 nil。
func Module() fx.Option {
	return fx.Module("otp",
		fx.Provide(newService),
	)
}

func newService(client *redis.Client, sender sms.Sender) *Service {
	if client == nil || sender == nil {
		return nil
	}

	return NewService(client, sender, Options{})
}
