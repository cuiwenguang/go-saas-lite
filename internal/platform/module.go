// Package platform 聚合平台控制台侧模块：平台登录 + 租户管理。
package platform

import (
	"go.uber.org/fx"

	"github.com/cuiwenguang/go-saas-lite/internal/platform/account"
	"github.com/cuiwenguang/go-saas-lite/internal/platform/audit"
	"github.com/cuiwenguang/go-saas-lite/internal/platform/plans"
	"github.com/cuiwenguang/go-saas-lite/internal/platform/stats"
	"github.com/cuiwenguang/go-saas-lite/internal/platform/tenants"
)

// Options 是平台侧的装配选项，由 internal/app 按进程配置传入。
type Options struct {
	// SecureCookies 为真时 refresh cookie 带 Secure（生产 HTTPS）。
	SecureCookies bool
}

// Module 聚合平台侧子模块（租户侧模块由 app 单独挂载，不在这里重复聚合）。
func Module(opts Options) fx.Option {
	return fx.Options(
		audit.Module(),
		account.Module(account.Options{SecureCookies: opts.SecureCookies}),
		tenants.Module(),
		plans.Module(),
		stats.Module(),
	)
}
