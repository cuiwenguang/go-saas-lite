package authz

import (
	"github.com/gin-gonic/gin"

	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/response"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// 平台侧角色：只有 owner / viewer 两个，不开放自定义，不进角色表（readme 2.3）。
const (
	PlatformRoleOwner  = "owner"
	PlatformRoleViewer = "viewer"
)

// 平台侧权限点，字符串与前端 web/src/shared/permission/perms.ts 同源。
var (
	TenantRead   = Read("tenant")
	TenantCreate = Create("tenant")
	TenantUpdate = Update("tenant")
	TenantDelete = Delete("tenant")

	PlanRead   = Read("plan")
	PlanUpdate = Update("plan")

	AuditRead = Read("audit")
	StatsRead = Read("stats")
)

// platformMatrix 是「角色 → 权限点」静态矩阵。
// 平台权限以代码为准：改这里即可，不落库、不做权限管理页。
var platformMatrix = map[string][]Perm{
	PlatformRoleOwner: {
		TenantRead, TenantCreate, TenantUpdate, TenantDelete,
		PlanRead, PlanUpdate, AuditRead, StatsRead,
	},
	PlatformRoleViewer: {
		TenantRead, PlanRead, AuditRead, StatsRead,
	},
}

// PlatformPermissions 返回角色的权限点字符串，供 GET /me 下发给前端控制菜单与按钮。
func PlatformPermissions(role string) []string {
	perms := platformMatrix[role]
	out := make([]string, 0, len(perms))
	for _, perm := range perms {
		out = append(out, perm.String())
	}

	return out
}

// PlatformAllows 判断角色是否拥有指定权限点。
func PlatformAllows(role string, perm Perm) bool {
	for _, granted := range platformMatrix[role] {
		if granted == perm {
			return true
		}
	}

	return false
}

// PlatformRequire 是平台侧权限中间件。
// 角色只取 auth.Guard 写入 ctx 的 Scope，不读请求体 / query / header。
func PlatformRequire(perm Perm) gin.HandlerFunc {
	return func(c *gin.Context) {
		scope, err := tenancy.ScopeFrom(c.Request.Context())
		if err != nil || scope.Actor != tenancy.ActorPlatform {
			response.Fail(c, apperr.New(apperr.CodePermissionDenied, "platform identity required"))
			return
		}
		if !PlatformAllows(scope.Role, perm) {
			response.Fail(c, apperr.New(apperr.CodePermissionDenied, "permission denied: "+perm.String()))
			return
		}

		c.Next()
	}
}
