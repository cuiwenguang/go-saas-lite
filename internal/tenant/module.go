// Package tenant 聚合租户侧模块：账号会话 + 成员 + 总览 + 租户配置。
package tenant

import (
	"go.uber.org/fx"

	"github.com/cuiwenguang/go-saas-lite/internal/tenant/account"
	"github.com/cuiwenguang/go-saas-lite/internal/tenant/audit"
	"github.com/cuiwenguang/go-saas-lite/internal/tenant/conf"
	"github.com/cuiwenguang/go-saas-lite/internal/tenant/file"
	"github.com/cuiwenguang/go-saas-lite/internal/tenant/members"
	"github.com/cuiwenguang/go-saas-lite/internal/tenant/stats"
)

// Options 是租户侧的装配选项，由 internal/app 按进程配置传入。
type Options struct {
	// SecureCookies 为真时 refresh cookie 带 Secure（生产 HTTPS）。
	SecureCookies bool
	// MaxUploadBytes 是单文件上传上限。
	MaxUploadBytes int64
}

// Module 聚合租户侧子模块。
func Module(opts Options) fx.Option {
	return fx.Options(
		account.Module(account.Options{SecureCookies: opts.SecureCookies}),
		audit.Module(),
		conf.Module(),
		file.Module(file.Options{MaxUploadBytes: opts.MaxUploadBytes}),
		members.Module(),
		stats.Module(),
	)
}
