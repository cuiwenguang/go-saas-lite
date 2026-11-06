package tenancy

import (
	"fmt"
	"sort"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 表名常量，避免各处手写字符串。
const (
	TableTenants            = "tenants"
	TablePlans              = "plans"
	TableUsageDaily         = "usage_daily"
	TablePlatformUsers      = "platform_users"
	TableOAuthClients       = "oauth_clients"
	TableAPIKeys            = "api_keys"
	TableSystemOperationLog = "system_operation_log"
	TableUserIdentities     = "user_identities"
)

// platformTables 是平台组白名单：这些表只能经 PlatformDB 访问，永不注入 tenant_id。
var platformTables = []string{
	TableAPIKeys,
	TableOAuthClients,
	TablePlans,
	TablePlatformUsers,
	TableSystemOperationLog,
	TableTenants,
	TableUsageDaily,
	TableUserIdentities,
}

// PlatformTables 返回平台组表名（拷贝，防止调用方修改）。
func PlatformTables() []string {
	out := make([]string, len(platformTables))
	copy(out, platformTables)
	return out
}

// IsPlatformTable 判断表是否属于平台组。
func IsPlatformTable(name string) bool {
	for _, table := range platformTables {
		if table == name {
			return true
		}
	}
	return false
}

// SortedPlatformTables 用于测试断言，保证稳定输出。
func SortedPlatformTables() []string {
	out := PlatformTables()
	sort.Strings(out)
	return out
}

type openOptions struct {
	tenantCallbacks bool
	silent          bool
}

// Option 调整连接行为。
type Option func(*openOptions)

// WithTenantCallbacks 为连接注册租户隔离 Callback（租户连接池使用）。
func WithTenantCallbacks() Option {
	return func(o *openOptions) { o.tenantCallbacks = true }
}

// Silent 关闭 GORM 默认日志（测试常用）。
func Silent() Option {
	return func(o *openOptions) { o.silent = true }
}

// OpenPostgres 建立数据库连接。
// 租户连接必须带 WithTenantCallbacks；平台连接不要带。
func OpenPostgres(dsn string, opts ...Option) (*gorm.DB, error) {
	cfg := openOptions{}
	for _, opt := range opts {
		opt(&cfg)
	}

	gormConfig := &gorm.Config{}
	if cfg.silent {
		gormConfig.Logger = logger.Default.LogMode(logger.Silent)
	}

	db, err := gorm.Open(postgres.Open(dsn), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	if cfg.tenantCallbacks {
		RegisterTenantCallbacks(db)
	}

	return db, nil
}
