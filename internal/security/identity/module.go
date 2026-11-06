package identity

import "go.uber.org/fx"

// Module 装配全局登录索引服务。
func Module() fx.Option {
	return fx.Module("identity",
		fx.Provide(NewService),
	)
}
