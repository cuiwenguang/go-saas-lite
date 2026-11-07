package app

import (
	"github.com/gin-gonic/gin"

	"github.com/cuiwenguang/go-saas-lite/internal/security/apikey"
	"github.com/cuiwenguang/go-saas-lite/internal/security/auth"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// TenantChain 是租户入口链（/api/tenant/v1）。
// 只接受 aud=tenant-api 的 tenant token；令牌有效后还要复查租户状态（到期读时判定）。
func TenantChain(group *gin.RouterGroup, issuer *auth.Issuer, lookup TenantLookup) *gin.RouterGroup {
	secured := group.Group("")
	secured.Use(auth.Guard(issuer, auth.Rule{
		Audience: auth.AudienceTenantAPI,
		Actor:    tenancy.ActorTenant,
	}))
	secured.Use(TenantStateGuard(lookup))
	secured.Use(PasswordChangeGuard())

	return secured
}

// PlatformChain 是平台入口链（/api/platform/v1）。
// 与租户链物理分开：平台 token 不能访问租户接口，反之亦然。
// 具体权限点由各接口用 authz.PlatformRequire 挂载。
func PlatformChain(group *gin.RouterGroup, issuer *auth.Issuer) *gin.RouterGroup {
	secured := group.Group("")
	secured.Use(auth.Guard(issuer, auth.Rule{
		Audience: auth.AudiencePlatformAPI,
		Actor:    tenancy.ActorPlatform,
	}))

	return secured
}

// MCPChain 是 Agent 入口链（/api/tenant/v1/mcp）。
// 只接受 aud=mcp 的 OAuth token 与用户级 API Key；Web 的 tenant-api token 不能调用。
// 鉴权通过后同样复查租户状态，避免停用 / 到期后 Agent 继续调用。
func MCPChain(group *gin.RouterGroup, issuer *auth.Issuer, keys *apikey.Service, lookup TenantLookup) *gin.RouterGroup {
	secured := group.Group("")
	secured.Use(MCPAuth(issuer, keys))
	secured.Use(TenantStateGuard(lookup))

	return secured
}
