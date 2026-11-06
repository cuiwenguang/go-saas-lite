package app

import (
	"context"
	"log/slog"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/obs"
	"github.com/cuiwenguang/go-saas-lite/pkg/response"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// tenantLookup 用平台库把域名 / 子域名 / ID 解析为租户。
type tenantLookup interface {
	ResolveTenant(ctx context.Context, key string) (tenantID string, status string, err error)
}

// TenantResolver 是免鉴权链路的租户解析：优先 X-Tenant-Id，其次子域名。
func TenantResolver(lookup tenantLookup, baseDomain string) gin.HandlerFunc {
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

		tenantID, status, err := lookup.ResolveTenant(c.Request.Context(), key)
		if err != nil {
			response.Fail(c, err)
			return
		}
		switch status {
		case "pending":
			response.Fail(c, apperr.New(apperr.CodeTenantNotReady, "tenant is not ready"))
			return
		case "suspended":
			response.Fail(c, apperr.New(apperr.CodeTenantDisabled, "tenant is disabled"))
			return
		case "expired":
			response.Fail(c, apperr.New(apperr.CodeTenantExpired, "tenant is expired"))
			return
		case "archived":
			response.Fail(c, apperr.New(apperr.CodeTenantNotFound, "tenant not found"))
			return
		}

		ctx := tenancy.WithScope(c.Request.Context(), tenancy.Scope{
			TenantID: tenantID,
			Actor:    tenancy.ActorTenant,
		})
		ctx = obs.WithLogAttrs(ctx, slog.String("tenant_id", tenantID))
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}
