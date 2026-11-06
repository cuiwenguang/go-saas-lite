// Package grant 提供用户 OAuth 授权的查看与撤销。
package grant

import "time"

// Grant 映射租户组 oauth_grants 表。
type Grant struct {
	TenantID  string     `gorm:"column:tenant_id"`
	UserID    string     `gorm:"column:user_id"`
	ClientID  string     `gorm:"column:client_id"`
	Scope     string     `gorm:"column:scope"`
	RevokedAt *time.Time `gorm:"column:revoked_at"`
	CreatedAt time.Time  `gorm:"column:created_at"`
	UpdatedAt time.Time  `gorm:"column:updated_at"`
}

// TableName 固定表名。
func (Grant) TableName() string { return "oauth_grants" }

// Client 映射平台组 oauth_clients 表（只取名）。
type Client struct {
	ClientID   string `gorm:"column:client_id;primaryKey"`
	ClientName string `gorm:"column:client_name"`
}

// TableName 固定表名。
func (Client) TableName() string { return "oauth_clients" }
