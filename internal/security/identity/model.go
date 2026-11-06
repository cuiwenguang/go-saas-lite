// Package identity 维护全局登录索引：登录标识 → (tenant_id, user_id)。
//
// 索引只做定位，不存密码与资料；租户库 users 是唯一真相源（readme 1.5）。
// 它让「邮箱/手机号全局唯一」与「共享表 + tenant_id 隔离」同时成立：
// 登录时先查索引拿到租户，再用 tenancy.DB 在该租户作用域内校验密码。
package identity

import "time"

// 标识类型。
const (
	TypeEmail = "email"
	TypePhone = "phone"
)

// Identity 映射平台组 user_identities 表。
type Identity struct {
	IdentifierType string    `gorm:"column:identifier_type;primaryKey"`
	Identifier     string    `gorm:"column:identifier;primaryKey"`
	TenantID       string    `gorm:"column:tenant_id"`
	UserID         string    `gorm:"column:user_id"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
}

// TableName 固定表名。
func (Identity) TableName() string { return "user_identities" }

// User 是租户组 users 表的只读投影，仅用于登录校验与对账。
type User struct {
	ID       string `gorm:"column:id;primaryKey"`
	TenantID string `gorm:"column:tenant_id"`
	Email    string `gorm:"column:email"`
	Status   string `gorm:"column:status"`
}

// TableName 固定表名。
func (User) TableName() string { return "users" }

// tenancyTenant 是平台组 tenants 表的只读投影，对账时用于枚举租户。
type tenancyTenant struct {
	ID        string    `gorm:"column:id;primaryKey"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

// TableName 固定表名。
func (tenancyTenant) TableName() string { return "tenants" }
