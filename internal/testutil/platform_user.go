package testutil

import "time"

// PlatformUser 映射平台组 platform_users 表。
type PlatformUser struct {
	ID           string `gorm:"column:id;primaryKey"`
	Email        string `gorm:"column:email"`
	PasswordHash string `gorm:"column:password_hash"`
	Role         string `gorm:"column:role"`
	Status       string `gorm:"column:status"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// TableName 固定表名。
func (PlatformUser) TableName() string { return "platform_users" }
