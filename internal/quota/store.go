package quota

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

const usageHashKey = "quota:usage"

// PlanStore 读取租户套餐与限额。
type PlanStore struct {
	platform tenancy.PlatformDB
}

// NewPlanStore 创建套餐读取器。
func NewPlanStore(platform tenancy.PlatformDB) *PlanStore {
	return &PlanStore{platform: platform}
}

// Limits 返回租户当前套餐的限额；套餐不存在时返回错误。
func (s *PlanStore) Limits(ctx context.Context, tenantID string) (Limits, error) {
	var tenant TenantPlan
	err := s.platform.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Where("id = ?", tenantID).First(&tenant)
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Limits{}, apperr.New(apperr.CodeTenantNotFound, "tenant not found")
	}
	if err != nil {
		return Limits{}, apperr.Wrap(err, apperr.CodeInternal, "load tenant plan")
	}

	var plan Plan
	err = s.platform.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Where("code = ?", tenant.PlanCode).First(&plan)
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Limits{}, apperr.New(apperr.CodePlanNotFound, "plan not found")
	}
	if err != nil {
		return Limits{}, apperr.Wrap(err, apperr.CodeInternal, "load plan")
	}

	limits, parseErr := ParseLimits(plan.Quota)
	if parseErr != nil {
		return Limits{}, apperr.Wrap(parseErr, apperr.CodeInternal, "parse plan quota")
	}

	return limits, nil
}

// Usage 是每日用量计数。
type Usage interface {
	Get(ctx context.Context, tenantID string, metric Metric, day time.Time) (int64, error)
	Incr(ctx context.Context, tenantID string, metric Metric, day time.Time, delta int64) (int64, error)
}

// RedisUsage 用 hash 存储每日计数，便于 worker 一次性落库。
type RedisUsage struct {
	client redis.UniversalClient
}

// NewRedisUsage 创建 Redis 计数器。
func NewRedisUsage(client redis.UniversalClient) *RedisUsage {
	return &RedisUsage{client: client}
}

func usageField(tenantID string, metric Metric, day time.Time) string {
	return strings.Join([]string{tenantID, string(metric), day.Format("2006-01-02")}, "|")
}

func (u *RedisUsage) Get(ctx context.Context, tenantID string, metric Metric, day time.Time) (int64, error) {
	value, err := u.client.HGet(ctx, usageHashKey, usageField(tenantID, metric, day)).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("quota: read usage: %w", err)
	}

	return value, nil
}

func (u *RedisUsage) Incr(ctx context.Context, tenantID string, metric Metric, day time.Time, delta int64) (int64, error) {
	value, err := u.client.HIncrBy(ctx, usageHashKey, usageField(tenantID, metric, day), delta).Result()
	if err != nil {
		return 0, fmt.Errorf("quota: incr usage: %w", err)
	}

	return value, nil
}
