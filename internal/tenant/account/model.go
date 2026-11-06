// Package account 提供租户用户会话、资料与 API Key 自助接口。
package account

import "time"

// User 映射租户组 users 表（本模块只投影所需列）。
type User struct {
	ID                 string    `gorm:"column:id;primaryKey"`
	TenantID           string    `gorm:"column:tenant_id"`
	Email              string    `gorm:"column:email"`
	DisplayName        string    `gorm:"column:display_name"`
	PasswordHash       string    `gorm:"column:password_hash"`
	Status             string    `gorm:"column:status"`
	MustChangePassword bool      `gorm:"column:must_change_password"`
	TokenVersion       int       `gorm:"column:token_version"`
	RefreshJTI         string    `gorm:"column:refresh_jti"`
	CreatedAt          time.Time `gorm:"column:created_at"`
}

// TableName 固定表名。
func (User) TableName() string { return "users" }

// Role 映射租户组 roles 表。
type Role struct {
	ID          string `gorm:"column:id;primaryKey"`
	TenantID    string `gorm:"column:tenant_id"`
	Code        string `gorm:"column:code"`
	Name        string `gorm:"column:name"`
	Permissions string `gorm:"column:permissions"`
}

// TableName 固定表名。
func (Role) TableName() string { return "roles" }

// UserRole 映射租户组 user_roles 表。
type UserRole struct {
	TenantID string `gorm:"column:tenant_id"`
	UserID   string `gorm:"column:user_id"`
	RoleID   string `gorm:"column:role_id"`
}

// TableName 固定表名。
func (UserRole) TableName() string { return "user_roles" }

// Tenant 是平台组 tenants 表的投影（/me 展示租户信息，登录时校验租户状态）。
type Tenant struct {
	ID        string     `gorm:"column:id;primaryKey"`
	Name      string     `gorm:"column:name"`
	Subdomain *string    `gorm:"column:subdomain"`
	PlanCode  string     `gorm:"column:plan_code"`
	Status    string     `gorm:"column:status"`
	ExpireAt  *time.Time `gorm:"column:expire_at"`
}

// TableName 固定表名。
func (Tenant) TableName() string { return "tenants" }
