package admin

import (
	"encoding/json"
	"time"
)

// CreateServerInput 是新建上游 MCP 服务入参。
type CreateServerInput struct {
	Name       string `json:"name"`
	BaseURL    string `json:"base_url"`
	Credential string `json:"credential"`
	Enabled    *bool  `json:"enabled"`
}

// UpdateServerInput 是可修改字段；nil 表示不改。
type UpdateServerInput struct {
	Name       *string `json:"name"`
	BaseURL    *string `json:"base_url"`
	Credential *string `json:"credential"`
	Enabled    *bool   `json:"enabled"`
}

// ServerOutput 是上游服务出参；凭据永不回显。
type ServerOutput struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	BaseURL       string    `json:"base_url"`
	HasCredential bool      `json:"has_credential"`
	Enabled       bool      `json:"enabled"`
	ToolCount     int64     `json:"tool_count"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// UpdateToolInput 只允许改启用状态与所需权限。
type UpdateToolInput struct {
	Enabled       *bool     `json:"enabled"`
	RequiredPerms *[]string `json:"required_perms"`
}

// ToolOutput 是工具出参。
type ToolOutput struct {
	ID            string          `json:"id"`
	ServerID      string          `json:"server_id"`
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	InputSchema   json.RawMessage `json:"input_schema"`
	RequiredPerms []string        `json:"required_perms"`
	Enabled       bool            `json:"enabled"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// CallFilter 是调用日志过滤条件。
type CallFilter struct {
	Status string
	Tool   string
	Page   int
	Size   int
}

// CallOutput 是调用日志出参。
type CallOutput struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	ClientID   string    `json:"client_id"`
	ToolName   string    `json:"tool_name"`
	Status     string    `json:"status"`
	DurationMS int       `json:"duration_ms"`
	ErrorCode  string    `json:"error_code"`
	CreatedAt  time.Time `json:"created_at"`
}

// PageOutput 是分页信封里的固定字段集合，由 handler 直接组装。
type PageOutput struct {
	Items any   `json:"items"`
	Total int64 `json:"total"`
	Page  int   `json:"page"`
	Size  int   `json:"size"`
}
