package audit

import "time"

// Log 映射租户组 tenant_operation_log 表。
type Log struct {
	ID         string    `gorm:"column:id;primaryKey"`
	TenantID   string    `gorm:"column:tenant_id"`
	ActorID    *string   `gorm:"column:actor_id"`
	ActorEmail string    `gorm:"column:actor_email"`
	Category   string    `gorm:"column:category"`
	Action     string    `gorm:"column:action"`
	Level      string    `gorm:"column:level"`
	Result     string    `gorm:"column:result"`
	ObjectType string    `gorm:"column:object_type"`
	ObjectID   string    `gorm:"column:object_id"`
	Summary    string    `gorm:"column:summary"`
	BeforeData *string   `gorm:"column:before_data"`
	AfterData  *string   `gorm:"column:after_data"`
	RequestID  string    `gorm:"column:request_id"`
	IP         string    `gorm:"column:ip"`
	CreatedAt  time.Time `gorm:"column:created_at"`
}

// TableName 固定表名。
func (Log) TableName() string { return "tenant_operation_log" }
