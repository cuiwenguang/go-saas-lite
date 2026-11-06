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

// LoginOutput 是登录结果。
type LoginOutput struct {
	AccessToken        string `json:"access_token"`
	TokenType          string `json:"token_type"`
	ExpiresIn          int    `json:"expires_in"`
	MustChangePassword bool   `json:"must_change_password"`
}
