package audit

// 审计分类。
const (
	CategorySecurity = "security"
	CategoryConfig   = "config"
	CategoryFile     = "file"
	CategoryOAuth    = "oauth"
)

// 动作常量。
const (
	ActionUserCreate        = "user.create"
	ActionUserUpdate        = "user.update"
	ActionUserResetPassword = "user.reset_password"
	ActionUserSetRoles      = "user.set_roles"

	ActionAPIKeyCreate = "apikey.create"
	ActionAPIKeyRevoke = "apikey.revoke"

	ActionMCPServerCreate = "mcp.server.create"
	ActionMCPServerUpdate = "mcp.server.update"
	ActionMCPServerDelete = "mcp.server.delete"
	ActionMCPToolUpdate   = "mcp.tool.update"

	ActionFileUpload = "file.upload"
	ActionFileDelete = "file.delete"

	ActionOAuthGrantRevoke = "oauth.grant.revoke"
)

// 级别。
const (
	LevelInfo = "info"
	LevelWarn = "warn"
)

// 结果。
const (
	ResultSuccess = "success"
	ResultFailed  = "failed"
)
