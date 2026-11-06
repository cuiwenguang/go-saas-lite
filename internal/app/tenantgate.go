package app

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/response"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// effectiveTenantStatus 是到期读时判定：active 且 expire_at 已过 → expired。
// 只做判定不写库，数据库状态由 worker 每日流转（readme 1.1）。
func effectiveTenantStatus(status string, expireAt *time.Time, now time.Time) string {
	if status == "active" && expireAt != nil && !expireAt.After(now) {
		return "expired"
	}

	return status
}

// tenantStateError 把租户状态映射为错误；nil 表示放行。
func tenantStateError(status string) error {
	switch status {
	case "pending":
		return apperr.New(apperr.CodeTenantNotReady, "tenant is not ready")
	case "suspended":
		return apperr.New(apperr.CodeTenantDisabled, "tenant is disabled")
	case "expired":
		return apperr.New(apperr.CodeTenantExpired, "tenant is expired")
	case "archived":
		return apperr.New(apperr.CodeTenantNotFound, "tenant not found")
	}

	return nil
}

// TenantStateGuard 是已鉴权链的二次状态复查（readme §九）：
// 令牌仍然有效，但租户已停用 / 到期 / 归档时同样拦截。
func TenantStateGuard(lookup TenantLookup) gin.HandlerFunc {
	return func(c *gin.Context) {
		scope, err := tenancy.ScopeFrom(c.Request.Context())
		if err != nil || scope.TenantID == "" {
			response.Fail(c, apperr.New(apperr.CodeUnauthenticated, "tenant identity required"))
			return
		}

		info, err := lookup.ResolveTenant(c.Request.Context(), scope.TenantID)
		if err != nil {
			response.Fail(c, err)
			return
		}

		if stateErr := tenantStateError(effectiveTenantStatus(info.Status, info.ExpireAt, time.Now())); stateErr != nil {
			response.Fail(c, stateErr)
			return
		}

		c.Next()
	}
}
