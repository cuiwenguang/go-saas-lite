package account

// LoginInput 是平台登录入参。
type LoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginOutput 是平台登录结果。
type LoginOutput struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Role        string `json:"role"`
	// RefreshToken 只走 httpOnly cookie，不进响应体。
	RefreshToken string `json:"-"`
}

// PlatformProfile 是 /me 返回的账号信息。
type PlatformProfile struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

// MeOutput 是 /me 的返回：前端据此渲染菜单与按钮，不按角色字符串判断。
type MeOutput struct {
	User        PlatformProfile `json:"user"`
	Role        string          `json:"role"`
	Permissions []string        `json:"permissions"`
}

// ChangePasswordInput 是平台账号改密入参。
type ChangePasswordInput struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}
