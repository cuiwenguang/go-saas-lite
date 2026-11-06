package audit

import "time"

// Record 映射平台组 system_operation_log 表。
type Record struct {
	ID         string    `gorm:"column:id;primaryKey"`
	Category   string    `gorm:"column:category"`
	Action     string    `gorm:"column:action"`
	Level      string    `gorm:"column:level"`
	Result     string    `gorm:"column:result"`
	ActorType  string    `gorm:"column:actor_type"`
	ActorID    *string   `gorm:"column:actor_id"`
	ActorEmail *string   `gorm:"column:actor_email"`
	ObjectType string    `gorm:"column:object_type"`
	ObjectID   string    `gorm:"column:object_id"`
	Summary    string    `gorm:"column:summary"`
	Before     *string   `gorm:"column:before_data"`
	After      *string   `gorm:"column:after_data"`
	RequestID  string    `gorm:"column:request_id"`
	IP         string    `gorm:"column:ip"`
	CreatedAt  time.Time `gorm:"column:created_at"`
}

// TableName 固定表名。
func (Record) TableName() string { return "system_operation_log" }
