// Package tenant 聚合租户侧模块：账号登录 + API Key 自助 + 租户配置。
package tenant

import (
	"go.uber.org/fx"

	"github.com/cuiwenguang/go-saas-lite/internal/tenant/account"
	"github.com/cuiwenguang/go-saas-lite/internal/tenant/conf"
)

// Module 聚合租户侧子模块。
func Module() fx.Option {
	return fx.Options(
		account.Module(),
		conf.Module(),
	)
}
