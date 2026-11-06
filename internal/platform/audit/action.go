package audit

// 审计分类：页面一级筛选入口，固定 6 类。
const (
	CategoryTenant   = "tenant"
	CategoryPlan     = "plan"
	CategoryAuth     = "auth"
	CategorySecurity = "security"
	CategoryConfig   = "config"
	CategorySystem   = "system"
)

// 动作是代码常量，业务代码禁止拼字符串（与权限点同一思路）。
const (
	ActionTenantCreate    = "tenant.create"
	ActionTenantUpdate    = "tenant.update"
	ActionTenantRenew     = "tenant.renew"
	ActionTenantStatus    = "tenant.update_status"
	ActionTenantArchive   = "tenant.archive"
	ActionTenantRetry     = "tenant.retry_provision"
	ActionTenantResetPass = "tenant.reset_admin_password"
	ActionTenantReadUsers = "tenant.cross_tenant_read"

	ActionPlanCreate = "plan.create"
	ActionPlanUpdate = "plan.update"
	ActionPlanToggle = "plan.toggle"

	ActionLogin          = "platform.login"
	ActionLoginFailed    = "platform.login_failed"
	ActionPasswordChange = "platform.password_change"

	ActionAutoExpire = "tenant.auto_expire"
)

// 级别：warn 供「只看风险操作」筛选用。
const (
	LevelInfo = "info"
	LevelWarn = "warn"
)

// 结果。
const (
	ResultSuccess = "success"
	ResultFailed  = "failed"
)

// 操作主体类型：admin 为人工操作，system 为 worker 自动动作。
const (
	ActorAdmin  = "admin"
	ActorSystem = "system"
)
