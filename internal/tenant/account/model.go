// Package account 提供租户用户登录与 API Key 自助接口。
package account

// User 映射租户组 users 表（本模块只投影所需列）。
type User struct {
	ID                 string `gorm:"column:id;primaryKey"`
	TenantID           string `gorm:"column:tenant_id"`
	Email              string `gorm:"column:email"`
	DisplayName        string `gorm:"column:display_name"`
	PasswordHash       string `gorm:"column:password_hash"`
	Status             string `gorm:"column:status"`
	MustChangePassword bool   `gorm:"column:must_change_password"`
	TokenVersion       int    `gorm:"column:token_version"`
}

// TableName 固定表名。
func (User) TableName() string { return "users" }
