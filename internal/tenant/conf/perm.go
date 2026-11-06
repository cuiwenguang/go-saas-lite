package conf

import "github.com/cuiwenguang/go-saas-lite/internal/security/authz"

// 权限点：读取对普通成员开放，写入仅管理员。
var (
	PermRead   = authz.Read("tenantconf")
	PermUpdate = authz.Update("tenantconf")
)

// Permissions 返回本模块声明的权限点，供启动时登记。
func Permissions() []authz.Perm {
	return []authz.Perm{PermRead, PermUpdate}
}
