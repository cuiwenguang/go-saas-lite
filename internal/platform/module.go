// Package platform 聚合平台控制台侧模块：平台登录 + 租户管理。
package platform

import (
	"go.uber.org/fx"

	"github.com/cuiwenguang/go-saas-lite/internal/platform/account"
	"github.com/cuiwenguang/go-saas-lite/internal/platform/tenant"
)

// Module 聚合平台侧子模块。
func Module() fx.Option {
	return fx.Options(
		account.Module(),
		tenant.Module(),
	)
}
