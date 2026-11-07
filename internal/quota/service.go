package quota

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// Service 负责配额校验、计数与落库。
type Service struct {
	platform tenancy.PlatformDB
	plans    *PlanStore
	usage    Usage
	client   redis.UniversalClient
	now      func() time.Time

	cacheMu sync.RWMutex
	cache   map[string]limitsCache
}

// NewService 创建配额服务。
func NewService(platform tenancy.PlatformDB, client redis.UniversalClient) *Service {
	return &Service{
		platform: platform,
		plans:    NewPlanStore(platform),
		usage:    NewRedisUsage(client),
		client:   client,
		now:      time.Now,
		cache:    make(map[string]limitsCache),
	}
}

// Limits 返回租户套餐限额（带 30 秒缓存）。
func (s *Service) Limits(ctx context.Context, tenantID string) (Limits, error) {
	return s.cachedLimits(ctx, tenantID)
}

// Check 只校验不计数。
func (s *Service) Check(ctx context.Context, tenantID string, metric Metric, delta int64) error {
	limits, err := s.cachedLimits(ctx, tenantID)
	if err != nil {
		return err
	}

	limit := limits.ForMetric(metric)
	if limit <= 0 {
		return nil
	}

	used, err := s.usage.Get(ctx, tenantID, metric, s.now())
	if err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "read quota usage")
	}

	if used+delta > limit {
		return apperr.Newf(apperr.CodeQuotaExceeded, "quota exceeded for %s", metric).
			WithMeta("metric", string(metric)).
			WithMeta("limit", limit).
			WithMeta("used", used)
	}

	return nil
}

// Consume 校验后累加，用于计次类指标。
func (s *Service) Consume(ctx context.Context, tenantID string, metric Metric, delta int64) error {
	if err := s.Check(ctx, tenantID, metric, delta); err != nil {
		return err
	}

	if _, err := s.usage.Incr(ctx, tenantID, metric, s.now(), delta); err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "incr quota usage")
	}

	return nil
}

// Incr 直接累加，不做上限校验，用于事后计量（如 mcp_tokens）。
func (s *Service) Incr(ctx context.Context, tenantID string, metric Metric, delta int64) error {
	if _, err := s.usage.Incr(ctx, tenantID, metric, s.now(), delta); err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "incr quota usage")
	}

	return nil
}

// CheckCount 校验「当前值」是否已触达上限，用于并发会话这类瞬时指标。
// current 为当前占用数；上限为 0 表示不限。
func (s *Service) CheckCount(ctx context.Context, tenantID string, metric Metric, current int64) error {
	limits, err := s.cachedLimits(ctx, tenantID)
	if err != nil {
		return err
	}

	limit := limits.ForMetric(metric)
	if limit <= 0 {
		return nil
	}

	if current >= limit {
		return apperr.Newf(apperr.CodeQuotaExceeded, "quota exceeded for %s", metric).
			WithMeta("metric", string(metric)).
			WithMeta("limit", limit).
			WithMeta("used", current)
	}

	return nil
}

// Used 返回当天已用量。
func (s *Service) Used(ctx context.Context, tenantID string, metric Metric) (int64, error) {
	used, err := s.usage.Get(ctx, tenantID, metric, s.now())
	if err != nil {
		return 0, apperr.Wrap(err, apperr.CodeInternal, "read quota usage")
	}
	return used, nil
}

// Flush 把 Redis 计数落到 usage_daily，并清理已处理的字段。
// 由 worker 周期调用；单次返回落库条数。
func (s *Service) Flush(ctx context.Context) (int, error) {
	fields, err := s.client.HGetAll(ctx, usageHashKey).Result()
	if err != nil {
		return 0, fmt.Errorf("quota: read usage hash: %w", err)
	}
	if len(fields) == 0 {
		return 0, nil
	}

	flushed := make([]string, 0, len(fields))
	for field, raw := range fields {
		tenantID, metric, day, ok := parseUsageField(field)
		if !ok {
			continue
		}

		var value int64
		if _, scanErr := fmt.Sscanf(raw, "%d", &value); scanErr != nil {
			continue
		}

		if err := s.upsertDaily(ctx, tenantID, metric, day, value); err != nil {
			return 0, err
		}
		flushed = append(flushed, field)
	}

	if len(flushed) > 0 {
		if err := s.client.HDel(ctx, usageHashKey, flushed...).Err(); err != nil {
			return 0, fmt.Errorf("quota: clear usage hash: %w", err)
		}
	}

	return len(flushed), nil
}

func (s *Service) upsertDaily(ctx context.Context, tenantID, metric, day string, value int64) error {
	err := s.platform.Tx(ctx, func(tx tenancy.Tx) error {
		return tx.Raw(`INSERT INTO usage_daily (tenant_id, metric, day, value)
			VALUES (?, ?, ?, ?)
			ON CONFLICT (tenant_id, metric, day)
			DO UPDATE SET value = EXCLUDED.value, updated_at = now()`,
			tenantID, metric, day, value).Scan(new(int64))
	})
	if err != nil {
		return fmt.Errorf("quota: flush %s/%s/%s: %w", tenantID, metric, day, err)
	}
	return nil
}

func parseUsageField(field string) (tenantID, metric, day string, ok bool) {
	parts := strings.Split(field, "|")
	if len(parts) != 3 {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}
