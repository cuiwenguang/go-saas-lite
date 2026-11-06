package app

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// TenantInfo 是解析出的租户状态快照；ExpireAt 参与到期读时判定。
type TenantInfo struct {
	ID       string
	Status   string
	ExpireAt *time.Time
}

// TenantLookup 把域名 / 子域名 / ID 解析为租户。
// 公开链（登录 / OAuth）与已鉴权链（状态复查）共用同一实现。
type TenantLookup interface {
	ResolveTenant(ctx context.Context, key string) (TenantInfo, error)
}

type tenantRecord struct {
	ID       string     `gorm:"column:id;primaryKey"`
	Status   string     `gorm:"column:status"`
	ExpireAt *time.Time `gorm:"column:expire_at"`
}

func (tenantRecord) TableName() string { return "tenants" }

// pgTenantLookup 用平台库解析租户（ID / 子域名 / 域名）。
type pgTenantLookup struct {
	platform tenancy.PlatformDB
}

// NewTenantLookup 创建租户解析器。
func NewTenantLookup(platform tenancy.PlatformDB) TenantLookup {
	return pgTenantLookup{platform: platform}
}

// ResolveTenant 实现 TenantLookup。
func (l pgTenantLookup) ResolveTenant(ctx context.Context, key string) (TenantInfo, error) {
	var row tenantRecord

	err := l.platform.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Where("id::text = ? OR subdomain = ? OR domain = ?", key, key, key).First(&row)
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return TenantInfo{}, apperr.New(apperr.CodeTenantNotFound, "tenant not found")
	}
	if err != nil {
		return TenantInfo{}, apperr.Wrap(err, apperr.CodeInternal, "resolve tenant")
	}

	return TenantInfo{ID: row.ID, Status: row.Status, ExpireAt: row.ExpireAt}, nil
}

// 保证接口实现。
var _ TenantLookup = pgTenantLookup{}
