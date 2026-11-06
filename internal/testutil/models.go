package testutil

import (
	"time"
)

// Tenant 映射平台组的 tenants 表。
type Tenant struct {
	ID        string `gorm:"column:id;primaryKey"`
	Code      string `gorm:"column:code"`
	Name      string `gorm:"column:name"`
	Status    string `gorm:"column:status"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableName 固定表名。
func (Tenant) TableName() string { return "tenants" }

// Widget 是用于验证隔离的最小租户组表。
type Widget struct {
	ID        string `gorm:"column:id;primaryKey"`
	TenantID  string `gorm:"column:tenant_id"`
	Name      string `gorm:"column:name"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableName 固定表名。
func (Widget) TableName() string { return "widgets" }

// Role 映射租户组的 roles 表。
type Role struct {
	ID          string `gorm:"column:id;primaryKey"`
	TenantID    string `gorm:"column:tenant_id"`
	Code        string `gorm:"column:code"`
	Name        string `gorm:"column:name"`
	Permissions string `gorm:"column:permissions"`
	IsBuiltin   bool   `gorm:"column:is_builtin"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// TableName 固定表名。
func (Role) TableName() string { return "roles" }

// UserRole 映射租户组的 user_roles 表。
type UserRole struct {
	TenantID  string `gorm:"column:tenant_id"`
	UserID    string `gorm:"column:user_id"`
	RoleID    string `gorm:"column:role_id"`
	CreatedAt time.Time
}

// TableName 固定表名。
func (UserRole) TableName() string { return "user_roles" }

// User 映射租户组的 users 表。
type User struct {
	ID                 string `gorm:"column:id;primaryKey"`
	TenantID           string `gorm:"column:tenant_id"`
	Email              string `gorm:"column:email"`
	DisplayName        string `gorm:"column:display_name"`
	PasswordHash       string `gorm:"column:password_hash"`
	Status             string `gorm:"column:status"`
	MustChangePassword bool   `gorm:"column:must_change_password"`
	TokenVersion       int    `gorm:"column:token_version"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// TableName 固定表名。
func (User) TableName() string { return "users" }
