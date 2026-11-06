package stats

import "time"

// User 映射租户组 users 表（只投影统计所需列）。
type User struct {
	ID       string `gorm:"column:id;primaryKey"`
	TenantID string `gorm:"column:tenant_id"`
	Status   string `gorm:"column:status"`
}

// TableName 固定表名。
func (User) TableName() string { return "users" }

// CallLog 映射租户组 mcp_call_log 表。
type CallLog struct {
	ID         string    `gorm:"column:id;primaryKey"`
	TenantID   string    `gorm:"column:tenant_id"`
	Status     string    `gorm:"column:status"`
	DurationMS int       `gorm:"column:duration_ms"`
	CreatedAt  time.Time `gorm:"column:created_at"`
}

// TableName 固定表名。
func (CallLog) TableName() string { return "mcp_call_log" }

// APIKey 映射平台组 api_keys 表（读侧只投影统计所需列）。
type APIKey struct {
	ID        string     `gorm:"column:id;primaryKey"`
	TenantID  string     `gorm:"column:tenant_id"`
	RevokedAt *time.Time `gorm:"column:revoked_at"`
}

// TableName 固定表名。
func (APIKey) TableName() string { return "api_keys" }

// PlanTenant 映射平台组 tenants 表，只取套餐编码。
type PlanTenant struct {
	ID       string `gorm:"column:id;primaryKey"`
	PlanCode string `gorm:"column:plan_code"`
}

// TableName 固定表名。
func (PlanTenant) TableName() string { return "tenants" }

// Plan 映射平台组 plans 表。
type Plan struct {
	Code  string `gorm:"column:code;primaryKey"`
	Name  string `gorm:"column:name"`
	Quota []byte `gorm:"column:quota"`
}

// TableName 固定表名。
func (Plan) TableName() string { return "plans" }
