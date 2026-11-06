// Package conf 管理租户级 KV 配置。
package conf

import "time"

// Config 映射租户组 tenant_configs 表。
type Config struct {
	TenantID  string    `gorm:"column:tenant_id"`
	Namespace string    `gorm:"column:namespace"`
	Key       string    `gorm:"column:key"`
	Value     string    `gorm:"column:value"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

// TableName 固定表名。
func (Config) TableName() string { return "tenant_configs" }
