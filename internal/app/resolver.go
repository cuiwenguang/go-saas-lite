package app

import (
	"log/slog"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/obs"
	"github.com/cuiwenguang/go-saas-lite/pkg/response"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// TenantResolver 是免鉴权链路的租户解析：优先 X-Tenant-Id，其次子域名。
func TenantResolver(lookup TenantLookup, baseDomain string) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader("X-Tenant-Id")
		if key == "" && baseDomain != "" {
			host := c.Request.Host
			if index := strings.LastIndex(host, ":"); index > 0 {
				host = host[:index]
			}
			host = strings.ToLower(host)
			suffix := "." + strings.ToLower(baseDomain)
			if strings.HasSuffix(host, suffix) {
				key = strings.TrimSuffix(host, suffix)
			}
		}
		if key == "" {
			response.Fail(c, apperr.New(apperr.CodeInvalidArgument, "tenant is required (X-Tenant-Id or subdomain)"))
			return
		}

		info, err := lookup.ResolveTenant(c.Request.Context(), key)
		if err != nil {
			response.Fail(c, err)
			return
		}
		if stateErr := tenantStateError(effectiveTenantStatus(info.Status, info.ExpireAt, time.Now())); stateErr != nil {
			response.Fail(c, stateErr)
			return
		}

		ctx := tenancy.WithScope(c.Request.Context(), tenancy.Scope{
			TenantID: info.ID,
			Actor:    tenancy.ActorTenant,
		})
		ctx = obs.WithLogAttrs(ctx, slog.String("tenant_id", info.ID))
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}
