package app

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

type tenantRecord struct {
	ID     string `gorm:"column:id;primaryKey"`
	Status string `gorm:"column:status"`
}

func (tenantRecord) TableName() string { return "tenants" }

// pgTenantLookup 用平台库解析租户（ID / 子域名 / 域名）。
type pgTenantLookup struct {
	platform tenancy.PlatformDB
}

// ResolveTenant 实现 tenantLookup。
func (l pgTenantLookup) ResolveTenant(ctx context.Context, key string) (string, string, error) {
	var row tenantRecord

	err := l.platform.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Where("id::text = ? OR subdomain = ? OR domain = ?", key, key, key).First(&row)
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", "", apperr.New(apperr.CodeTenantNotFound, "tenant not found")
	}
	if err != nil {
		return "", "", apperr.Wrap(err, apperr.CodeInternal, "resolve tenant")
	}

	return row.ID, row.Status, nil
}

// 保证接口实现。
var _ tenantLookup = pgTenantLookup{}

// expireAt 便于测试构造过期时间。
func expireAt(d time.Duration) time.Time { return time.Now().Add(d) }
