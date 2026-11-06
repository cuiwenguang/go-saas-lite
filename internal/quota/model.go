package quota

// Plan 映射平台组 plans 表。
type Plan struct {
	ID      string `gorm:"column:id;primaryKey"`
	Code    string `gorm:"column:code"`
	Name    string `gorm:"column:name"`
	Quota   string `gorm:"column:quota"`
	Enabled bool   `gorm:"column:enabled"`
}

// TableName 固定表名。
func (Plan) TableName() string { return "plans" }

// TenantPlan 只取租户的套餐编码（tenants 表的投影）。
type TenantPlan struct {
	ID       string `gorm:"column:id;primaryKey"`
	PlanCode string `gorm:"column:plan_code"`
}

// TableName 固定表名。
func (TenantPlan) TableName() string { return "tenants" }
