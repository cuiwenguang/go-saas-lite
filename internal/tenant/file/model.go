package file

import "time"

// File 映射租户组 tenant_files 表。
type File struct {
	ID           string    `gorm:"column:id;primaryKey"`
	TenantID     string    `gorm:"column:tenant_id"`
	Key          string    `gorm:"column:key"`
	Biz          string    `gorm:"column:biz"`
	OriginalName string    `gorm:"column:original_name"`
	ContentType  string    `gorm:"column:content_type"`
	SizeBytes    int64     `gorm:"column:size_bytes"`
	CreatedAt    time.Time `gorm:"column:created_at"`
}

// TableName 固定表名。
func (File) TableName() string { return "tenant_files" }
