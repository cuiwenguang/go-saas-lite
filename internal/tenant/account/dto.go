package account

// LoginInput 是租户登录入参。
type LoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// CreateKeyInput 是创建 API Key 入参。
type CreateKeyInput struct {
	Name string `json:"name"`
}

// LoginOutput 是登录 / 刷新结果；refresh 令牌只走 httpOnly cookie。
type LoginOutput struct {
	AccessToken        string `json:"access_token"`
	TokenType          string `json:"token_type"`
	ExpiresIn          int    `json:"expires_in"`
	MustChangePassword bool   `json:"must_change_password"`
	RefreshToken       string `json:"-"`
}

// Profile 是 /me 的用户信息。
type Profile struct {
	ID          string   `json:"id"`
	Email       string   `json:"email"`
	DisplayName string   `json:"display_name"`
	Roles       []string `json:"roles"`
}

// TenantProfile 是 /me 的租户信息。
type TenantProfile struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Subdomain string `json:"subdomain,omitempty"`
	PlanCode  string `json:"plan_code"`
}

// MeOutput 是 /me 的返回：前端据此渲染菜单与按钮。
type MeOutput struct {
	User        Profile       `json:"user"`
	Tenant      TenantProfile `json:"tenant"`
	Permissions []string      `json:"permissions"`
}

// ChangePasswordInput 是租户用户改密入参。
type ChangePasswordInput struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}
