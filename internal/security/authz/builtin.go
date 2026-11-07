package authz

// 内置角色的权限定义。内置角色以代码为准，DB 只存结果（readme 2.3）。
const (
	RoleAdmin  = "admin"
	RoleMember = "member"
)

// API Key 自助管理的权限点。
var (
	PermAPIKeyRead   = Read("apikey")
	PermAPIKeyCreate = Create("apikey")
	PermAPIKeyRevoke = Custom("apikey", "revoke")
)

// 租户控制台其余权限点。
var (
	PermUserRead   = Read("user")
	PermUserCreate = Create("user")
	PermUserUpdate = Update("user")
	PermUserDelete = Delete("user")
	PermRoleRead   = Read("role")

	PermMCPRead   = Read("mcp")
	PermMCPUpdate = Update("mcp")

	PermStatsRead = Read("stats")

	PermFileRead   = Read("file")
	PermFileCreate = Create("file")
	PermFileDelete = Delete("file")

	PermOAuthGrantRead   = Read("oauth.grant")
	PermOAuthGrantRevoke = Custom("oauth.grant", "revoke")
)

// MemberPerms 是普通成员的最小权限集。
// 权限点全部来自代码常量，新增时必须同时在此登记。
var MemberPerms = []Perm{
	Read("profile"),
	Update("profile"),
	Read("tenantconf"),
	PermUserRead,
	PermRoleRead,
	PermAPIKeyRead,
	PermAPIKeyCreate,
	PermAPIKeyRevoke,
	PermMCPRead,
	PermStatsRead,
	PermFileRead,
	PermFileCreate,
}

// BuiltinRoles 返回内置角色 → 权限集合。
func BuiltinRoles() map[string][]Perm {
	return map[string][]Perm{
		RoleAdmin:  {Wildcard()},
		RoleMember: MemberPerms,
	}
}

// RegisterBuiltins 把内置角色与租户控制台用到的权限点登记到全局表。
func RegisterBuiltins() error {
	perms := make([]Perm, 0, len(MemberPerms)+8)
	perms = append(perms, Wildcard())
	perms = append(perms, MemberPerms...)
	perms = append(perms,
		PermUserCreate, PermUserUpdate, PermUserDelete,
		PermMCPUpdate,
		PermFileDelete,
		PermOAuthGrantRead, PermOAuthGrantRevoke,
		Read("audit"),
	)

	return Register(perms...)
}
