package quota

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/response"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

type limitsCache struct {
	limits  Limits
	expires time.Time
}

// qpsKey 按秒分桶，避免长窗口误差。
func qpsKey(tenantID string, at time.Time) string {
	return fmt.Sprintf("tenant:%s:quota:qps:%d", tenantID, at.Unix())
}

// ConsumeQPS 用 Redis 计数做每秒限流；api_qps<=0 表示不限。
func (s *Service) ConsumeQPS(ctx context.Context, tenantID string) error {
	limits, err := s.Limits(ctx, tenantID)
	if err != nil {
		return err
	}
	if limits.APIQPS <= 0 {
		return nil
	}

	key := qpsKey(tenantID, s.now())
	count, err := s.client.Incr(ctx, key).Result()
	if err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "incr qps")
	}
	if count == 1 {
		_ = s.client.Expire(ctx, key, 2*time.Second).Err()
	}
	if count > int64(limits.APIQPS) {
		return apperr.Newf(apperr.CodeQuotaExceeded, "rate limit exceeded").
			WithMeta("limit", limits.APIQPS)
	}

	return nil
}

// Middleware 在租户链上做每秒限流与 api 调用计数。
func Middleware(service *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if service == nil {
			c.Next()
			return
		}

		scope, err := tenancy.ScopeFrom(c.Request.Context())
		if err != nil {
			c.Next()
			return
		}

		if qpsErr := service.ConsumeQPS(c.Request.Context(), scope.TenantID); qpsErr != nil {
			response.Fail(c, qpsErr)
			return
		}

		if _, incrErr := service.usage.Incr(c.Request.Context(), scope.TenantID, MetricAPICalls, service.now(), 1); incrErr != nil {
			c.Next()
			return
		}

		c.Next()
	}
}

// cachedLimits 带 30 秒缓存，避免每请求查套餐。
func (s *Service) cachedLimits(ctx context.Context, tenantID string) (Limits, error) {
	s.cacheMu.RLock()
	entry, ok := s.cache[tenantID]
	s.cacheMu.RUnlock()
	if ok && s.now().Before(entry.expires) {
		return entry.limits, nil
	}

	limits, err := s.plans.Limits(ctx, tenantID)
	if err != nil {
		return Limits{}, err
	}

	s.cacheMu.Lock()
	s.cache[tenantID] = limitsCache{limits: limits, expires: s.now().Add(30 * time.Second)}
	s.cacheMu.Unlock()

	return limits, nil
}
