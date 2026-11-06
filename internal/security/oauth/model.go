package oauth

import "time"

// OAuthClient 映射平台组 oauth_clients 表。
type OAuthClient struct {
	ClientID     string     `gorm:"column:client_id;primaryKey"`
	TenantID     string     `gorm:"column:tenant_id"`
	ClientName   string     `gorm:"column:client_name"`
	RedirectURIs string     `gorm:"column:redirect_uris"`
	GrantTypes   string     `gorm:"column:grant_types"`
	RevokedAt    *time.Time `gorm:"column:revoked_at"`
	CreatedAt    time.Time  `gorm:"column:created_at"`
}

// TableName 固定表名。
func (OAuthClient) TableName() string { return "oauth_clients" }

// OAuthGrant 映射租户组 oauth_grants 表。
type OAuthGrant struct {
	TenantID  string     `gorm:"column:tenant_id"`
	UserID    string     `gorm:"column:user_id"`
	ClientID  string     `gorm:"column:client_id"`
	Scope     string     `gorm:"column:scope"`
	RevokedAt *time.Time `gorm:"column:revoked_at"`
	CreatedAt time.Time  `gorm:"column:created_at"`
	UpdatedAt time.Time  `gorm:"column:updated_at"`
}

// TableName 固定表名。
func (OAuthGrant) TableName() string { return "oauth_grants" }
