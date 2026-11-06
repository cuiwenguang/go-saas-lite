package tenants

import "time"

// 租户状态机（readme D5）。
const (
	StatusPending   = "pending"
	StatusActive    = "active"
	StatusSuspended = "suspended"
	StatusExpired   = "expired"
	StatusArchived  = "archived"
)

// 开通进度。
const (
	ProvisionPending   = "pending"
	ProvisionRunning   = "running"
	ProvisionSucceeded = "succeeded"
	ProvisionFailed    = "failed"
)

// Tenant 映射平台组 tenants 表。
type Tenant struct {
	ID              string     `gorm:"column:id;primaryKey" json:"id"`
	Code            string     `gorm:"column:code" json:"code"`
	Name            string     `gorm:"column:name" json:"name"`
	Domain          *string    `gorm:"column:domain" json:"domain,omitempty"`
	Subdomain       *string    `gorm:"column:subdomain" json:"subdomain,omitempty"`
	Status          string     `gorm:"column:status" json:"status"`
	PlanCode        string     `gorm:"column:plan_code" json:"plan_code"`
	ExpireAt        *time.Time `gorm:"column:expire_at" json:"expire_at,omitempty"`
	Timezone        string     `gorm:"column:timezone" json:"timezone"`
	Locale          string     `gorm:"column:locale" json:"locale"`
	ContactName     *string    `gorm:"column:contact_name" json:"contact_name,omitempty"`
	ContactPhone    *string    `gorm:"column:contact_phone" json:"contact_phone,omitempty"`
	ContactEmail    *string    `gorm:"column:contact_email" json:"contact_email,omitempty"`
	AdminEmail      *string    `gorm:"column:admin_email" json:"admin_email,omitempty"`
	ProvisionStatus string     `gorm:"column:provision_status" json:"provision_status"`
	ProvisionStep   *string    `gorm:"column:provision_step" json:"provision_step,omitempty"`
	// ProvisionLog 存 jsonb 数组，由 service 解析成 []ProvisionEvent 输出。
	ProvisionLog string     `gorm:"column:provision_log" json:"-"`
	DeletedAt    *time.Time `gorm:"column:deleted_at" json:"-"`
	CreatedAt    time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt    time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

// TableName 固定表名。
func (Tenant) TableName() string { return "tenants" }

// allowedTransitions 是状态流转白名单。
var allowedTransitions = map[string][]string{
	StatusPending:   {StatusActive, StatusArchived},
	StatusActive:    {StatusSuspended, StatusExpired, StatusArchived},
	StatusSuspended: {StatusActive, StatusExpired, StatusArchived},
	StatusExpired:   {StatusActive, StatusArchived},
	StatusArchived:  {},
}

func canTransition(from, to string) bool {
	for _, allowed := range allowedTransitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

// Role 映射租户组 roles 表。
type Role struct {
	ID          string    `gorm:"column:id;primaryKey"`
	TenantID    string    `gorm:"column:tenant_id"`
	Code        string    `gorm:"column:code"`
	Name        string    `gorm:"column:name"`
	Permissions string    `gorm:"column:permissions"`
	IsBuiltin   bool      `gorm:"column:is_builtin"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

// TableName 固定表名。
func (Role) TableName() string { return "roles" }

// User 映射租户组 users 表（开通流水线只投影所需列）。
type User struct {
	ID                 string    `gorm:"column:id;primaryKey"`
	TenantID           string    `gorm:"column:tenant_id"`
	Email              string    `gorm:"column:email"`
	DisplayName        string    `gorm:"column:display_name"`
	PasswordHash       string    `gorm:"column:password_hash"`
	Status             string    `gorm:"column:status"`
	MustChangePassword bool      `gorm:"column:must_change_password"`
	TokenVersion       int       `gorm:"column:token_version"`
	CreatedAt          time.Time `gorm:"column:created_at"`
	UpdatedAt          time.Time `gorm:"column:updated_at"`
}

// TableName 固定表名。
func (User) TableName() string { return "users" }

// UserRole 映射租户组 user_roles 表。
type UserRole struct {
	TenantID  string    `gorm:"column:tenant_id"`
	UserID    string    `gorm:"column:user_id"`
	RoleID    string    `gorm:"column:role_id"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

// TableName 固定表名。
func (UserRole) TableName() string { return "user_roles" }
