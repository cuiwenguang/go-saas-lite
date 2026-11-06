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

// MemberPerms 是普通成员的最小权限集。
// 权限点全部来自代码常量，新增时必须同时在此登记。
var MemberPerms = []Perm{
	Read("profile"),
	Update("profile"),
	Read("tenantconf"),
	Read("user"),
	PermAPIKeyRead,
	PermAPIKeyCreate,
	PermAPIKeyRevoke,
}

// BuiltinRoles 返回内置角色 → 权限集合。
func BuiltinRoles() map[string][]Perm {
	return map[string][]Perm{
		RoleAdmin:  {Wildcard()},
		RoleMember: MemberPerms,
	}
}

// RegisterBuiltins 把内置角色用到的权限点登记到全局表。
func RegisterBuiltins() error {
	perms := make([]Perm, 0, len(MemberPerms)+1)
	perms = append(perms, Wildcard())
	perms = append(perms, MemberPerms...)

	return Register(perms...)
}
