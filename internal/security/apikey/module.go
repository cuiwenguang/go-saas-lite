package apikey

import "go.uber.org/fx"

// Module 装配 API Key 能力（无 HTTP 入口）。
func Module() fx.Option {
	return fx.Module("apikey",
		fx.Provide(NewService),
	)
}
