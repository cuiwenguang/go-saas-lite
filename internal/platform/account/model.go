// Package account 提供平台管理员登录与平台侧角色校验。
package account

import "github.com/cuiwenguang/go-saas-lite/internal/security/authz"

// 平台角色只有 owner / viewer，矩阵定义在 authz.platformMatrix。
const (
	RoleOwner  = authz.PlatformRoleOwner
	RoleViewer = authz.PlatformRoleViewer
)

// PlatformUser 映射平台组 platform_users 表。
type PlatformUser struct {
	ID           string `gorm:"column:id;primaryKey"`
	Email        string `gorm:"column:email"`
	PasswordHash string `gorm:"column:password_hash"`
	Role         string `gorm:"column:role"`
	Status       string `gorm:"column:status"`
	// RefreshJTI 是当前有效的 refresh 令牌 id，只保留最近一次（单会话）。
	RefreshJTI string `gorm:"column:refresh_jti"`
}

// TableName 固定表名。
func (PlatformUser) TableName() string { return "platform_users" }
