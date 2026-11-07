package tenants

import "time"

// CreateInput 是开通租户的入参。
type CreateInput struct {
	Code             string `json:"code"`
	Name             string `json:"name"`
	Timezone         string `json:"timezone"`
	Locale           string `json:"locale"`
	Domain           string `json:"domain"`
	Subdomain        string `json:"subdomain"`
	PlanCode         string `json:"plan_code"`
	ContactName      string `json:"contact_name"`
	ContactPhone     string `json:"contact_phone"`
	ContactEmail     string `json:"contact_email"`
	AdminEmail       string `json:"admin_email"`
	AdminPhone       string `json:"admin_phone"`
	AdminDisplayName string `json:"admin_display_name"`
	AdminPassword    string `json:"admin_password"`
}

// ListFilter 是列表过滤条件。
type ListFilter struct {
	Status       string
	Keyword      string
	PlanCode     string
	ExpireBefore *time.Time
	Page         int
	Size         int
}

// UpdateInput 是可修改字段；字段为 nil 表示不改。
type UpdateInput struct {
	Name         *string    `json:"name"`
	PlanCode     *string    `json:"plan_code"`
	ExpireAt     *time.Time `json:"expire_at"`
	Timezone     *string    `json:"timezone"`
	Locale       *string    `json:"locale"`
	ContactName  *string    `json:"contact_name"`
	ContactPhone *string    `json:"contact_phone"`
	ContactEmail *string    `json:"contact_email"`
}

// UpdateStatusInput 是变更租户状态的入参。
type UpdateStatusInput struct {
	Status string `json:"status"`
}

// ProvisionEvent 是开通流水线的一步记录。
type ProvisionEvent struct {
	Step  string    `json:"step"`
	OK    bool      `json:"ok"`
	Error string    `json:"error,omitempty"`
	At    time.Time `json:"at"`
}

// SummaryOutput 是列表分段筛选的计数。
type SummaryOutput struct {
	Total     int64 `json:"total"`
	Pending   int64 `json:"pending"`
	Active    int64 `json:"active"`
	Suspended int64 `json:"suspended"`
	Expired   int64 `json:"expired"`
	Archived  int64 `json:"archived"`
}

// CredentialOutput 携带一次性初始密码（仅创建 / 重置响应返回一次）。
type CredentialOutput struct {
	Tenant          TenantOutput `json:"tenant"`
	InitialPassword string       `json:"initial_password"`
}

// TenantUserOutput 是租户详情里的用户行。
type TenantUserOutput struct {
	ID                 string    `json:"id"`
	Email              string    `json:"email"`
	Phone              string    `json:"phone,omitempty"`
	DisplayName        string    `json:"display_name"`
	Status             string    `json:"status"`
	Roles              []string  `json:"roles"`
	MustChangePassword bool      `json:"must_change_password"`
	CreatedAt          time.Time `json:"created_at"`
}

// UsageCount 是租户用量快照（窗口口径见 UsageOutput）。
type UsageCount struct {
	Users     int64 `json:"users"`
	MCPCalls  int64 `json:"mcp_calls"`
	APICalls  int64 `json:"api_calls"`
	MCPTokens int64 `json:"mcp_tokens"`
}

// UsagePoint 是趋势图上的一点。
type UsagePoint struct {
	Date  string `json:"date"`
	Value int64  `json:"value"`
}

// UsageOutput 是租户详情「配额与用量」的数据：限额来自当前套餐，用量为近 30 天累计。
type UsageOutput struct {
	PlanCode    string       `json:"plan_code"`
	MaxUsers    int          `json:"max_users"`
	MCPCallsCap int64        `json:"mcp_calls_cap"`
	MCPTokenCap int64        `json:"mcp_tokens_cap"`
	APICallQPS  int          `json:"api_qps"`
	Usage       UsageCount   `json:"usage"`
	Series      []UsagePoint `json:"series"`
}

// TenantOutput 是租户出参，与表结构解耦，避免模型改动直接透传到前端。
type TenantOutput struct {
	ID              string           `json:"id"`
	Code            string           `json:"code"`
	Name            string           `json:"name"`
	Domain          string           `json:"domain,omitempty"`
	Subdomain       string           `json:"subdomain,omitempty"`
	Status          string           `json:"status"`
	PlanCode        string           `json:"plan_code"`
	ExpireAt        *time.Time       `json:"expire_at,omitempty"`
	Timezone        string           `json:"timezone"`
	Locale          string           `json:"locale"`
	ContactName     string           `json:"contact_name,omitempty"`
	ContactPhone    string           `json:"contact_phone,omitempty"`
	ContactEmail    string           `json:"contact_email,omitempty"`
	AdminEmail      string           `json:"admin_email,omitempty"`
	AdminPhone      string           `json:"admin_phone,omitempty"`
	ProvisionStatus string           `json:"provision_status"`
	ProvisionStep   string           `json:"provision_step,omitempty"`
	ProvisionLog    []ProvisionEvent `json:"provision_log"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
}
