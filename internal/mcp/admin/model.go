package admin

import "time"

// Server 映射租户组 mcp_servers 表。
type Server struct {
	ID         string    `gorm:"column:id;primaryKey"`
	TenantID   string    `gorm:"column:tenant_id"`
	Name       string    `gorm:"column:name"`
	BaseURL    string    `gorm:"column:base_url"`
	Credential string    `gorm:"column:credential"`
	Enabled    bool      `gorm:"column:enabled"`
	CreatedAt  time.Time `gorm:"column:created_at"`
	UpdatedAt  time.Time `gorm:"column:updated_at"`
}

// TableName 固定表名。
func (Server) TableName() string { return "mcp_servers" }

// Tool 映射租户组 mcp_tools 表。
type Tool struct {
	ID            string    `gorm:"column:id;primaryKey"`
	TenantID      string    `gorm:"column:tenant_id"`
	ServerID      string    `gorm:"column:server_id"`
	Name          string    `gorm:"column:name"`
	Description   string    `gorm:"column:description"`
	InputSchema   string    `gorm:"column:input_schema"`
	RequiredPerms string    `gorm:"column:required_perms"`
	Enabled       bool      `gorm:"column:enabled"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

// TableName 固定表名。
func (Tool) TableName() string { return "mcp_tools" }

// CallLog 映射租户组 mcp_call_log 表。
type CallLog struct {
	ID         string    `gorm:"column:id;primaryKey"`
	TenantID   string    `gorm:"column:tenant_id"`
	UserID     string    `gorm:"column:user_id"`
	ClientID   string    `gorm:"column:client_id"`
	ToolName   string    `gorm:"column:tool_name"`
	Status     string    `gorm:"column:status"`
	DurationMS int       `gorm:"column:duration_ms"`
	ErrorCode  string    `gorm:"column:error_code"`
	CreatedAt  time.Time `gorm:"column:created_at"`
}

// TableName 固定表名。
func (CallLog) TableName() string { return "mcp_call_log" }
