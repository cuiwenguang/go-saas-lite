// Package account 提供平台管理员登录与平台侧角色校验。
package account

// 平台角色：owner 可写，viewer 只读。
const (
	RoleOwner  = "owner"
	RoleViewer = "viewer"
)

// PlatformUser 映射平台组 platform_users 表。
type PlatformUser struct {
	ID           string `gorm:"column:id;primaryKey"`
	Email        string `gorm:"column:email"`
	PasswordHash string `gorm:"column:password_hash"`
	Role         string `gorm:"column:role"`
	Status       string `gorm:"column:status"`
}

// TableName 固定表名。
func (PlatformUser) TableName() string { return "platform_users" }
