// Package apikey 管理用户级 API Key（平台组表，鉴权前可查）。
package apikey

import "time"

// KeyPrefix 是明文前缀，便于识别与展示。
const KeyPrefix = "sk_live_"

// APIKey 映射平台组 api_keys 表。
type APIKey struct {
	ID         string     `gorm:"column:id;primaryKey"`
	TenantID   string     `gorm:"column:tenant_id"`
	UserID     string     `gorm:"column:user_id"`
	Name       string     `gorm:"column:name"`
	KeyPrefix  string     `gorm:"column:key_prefix"`
	KeyHash    string     `gorm:"column:key_hash"`
	RevokedAt  *time.Time `gorm:"column:revoked_at"`
	LastUsedAt *time.Time `gorm:"column:last_used_at"`
	CreatedAt  time.Time  `gorm:"column:created_at"`
}

// TableName 固定表名。
func (APIKey) TableName() string { return "api_keys" }
