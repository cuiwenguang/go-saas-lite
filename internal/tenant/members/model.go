package members

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
