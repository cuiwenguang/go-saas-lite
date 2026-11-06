package stats

import (
	"context"
	"encoding/json"
	"time"

	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// Service 提供租户总览聚合：成员、套餐限额、近 30 天用量与近 7 天调用趋势。
type Service struct {
	db       tenancy.DB
	platform tenancy.PlatformDB
}

// NewService 创建统计服务。
func NewService(db tenancy.DB, platform tenancy.PlatformDB) *Service {
	return &Service{db: db, platform: platform}
}

// Overview 汇总当前租户的总览数据。
func (s *Service) Overview(ctx context.Context) (*OverviewOutput, error) {
	scope, err := tenancy.ScopeFrom(ctx)
	if err != nil || scope.TenantID == "" {
		return nil, apperr.New(apperr.CodeUnauthenticated, "tenant identity required")
	}

	now := time.Now()
	since30 := now.AddDate(0, 0, -30)
	since7 := now.AddDate(0, 0, -6)

	var members MemberStats
	var agg struct {
		Total    int64 `gorm:"column:total"`
		Failed   int64 `gorm:"column:failed"`
		AvgMilli int64 `gorm:"column:avg_milli"`
	}
	var dayRows []dayCountRow

	if err := s.db.Read(ctx, func(tx tenancy.Tx) error {
		if err := tx.Model(&User{}).Count(&members.Total); err != nil {
			return err
		}
		if err := tx.Model(&User{}).Where("status = ?", "active").Count(&members.Active); err != nil {
			return err
		}
		if err := tx.Raw(callAggSQL, scope.TenantID, since30).Scan(&agg); err != nil {
			return err
		}

		return tx.Raw(callSeriesSQL, scope.TenantID, since7).Scan(&dayRows)
	}); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "tenant stats overview")
	}

	plan, err := s.planOf(ctx, scope.TenantID)
	if err != nil {
		return nil, err
	}
	activeKeys, metricTotals, err := s.platformUsage(ctx, scope.TenantID, since30)
	if err != nil {
		return nil, err
	}

	return &OverviewOutput{
		Members: members,
		Plan:    plan,
		Usage: UsageStats{
			Calls30d:      agg.Total,
			Failed30d:     agg.Failed,
			AvgDurationMS: agg.AvgMilli,
			ActiveAPIKeys: activeKeys,
			MetricTotals:  metricTotals,
		},
		CallSeries: buildSeries(now, dayRows2map(dayRows)),
	}, nil
}

// planOf 读取租户套餐与限额；平台组查询必须显式带 tenant_id。
func (s *Service) planOf(ctx context.Context, tenantID string) (PlanStats, error) {
	out := PlanStats{Quota: map[string]any{}}

	var tenant PlanTenant
	var plan Plan

	err := s.platform.Read(ctx, func(tx tenancy.Tx) error {
		if err := tx.Model(&PlanTenant{}).Select("id", "plan_code").Where("id = ?", tenantID).First(&tenant); err != nil {
			return err
		}

		return tx.Model(&Plan{}).Where("code = ?", tenant.PlanCode).First(&plan)
	})
	if err != nil {
		return out, apperr.Wrap(err, apperr.CodeInternal, "load tenant plan")
	}

	if len(plan.Quota) > 0 {
		_ = json.Unmarshal(plan.Quota, &out.Quota)
	}
	out.Code = plan.Code
	out.Name = plan.Name
	if v, ok := out.Quota["max_users"].(float64); ok {
		out.MaxUsers = int64(v)
	}

	return out, nil
}

// platformUsage 读取有效 API Key 数量与近 30 天用量快照。
func (s *Service) platformUsage(ctx context.Context, tenantID string, since time.Time) (int64, map[string]int64, error) {
	var keys struct {
		Total int64 `gorm:"column:total"`
	}
	var rows []struct {
		Metric string `gorm:"column:metric"`
		Total  int64  `gorm:"column:total"`
	}

	err := s.platform.Read(ctx, func(tx tenancy.Tx) error {
		if err := tx.Raw(activeKeysSQL, tenantID).Scan(&keys); err != nil {
			return err
		}

		return tx.Raw(usageTotalsSQL, tenantID, since).Scan(&rows)
	})
	if err != nil {
		return 0, nil, apperr.Wrap(err, apperr.CodeInternal, "load tenant usage")
	}

	totals := make(map[string]int64, len(rows))
	for _, row := range rows {
		totals[row.Metric] = row.Total
	}

	return keys.Total, totals, nil
}

type dayCountRow struct {
	Day   time.Time `gorm:"column:day"`
	Total int64     `gorm:"column:total"`
}

func dayRows2map(rows []dayCountRow) map[string]int64 {
	out := make(map[string]int64, len(rows))
	for _, row := range rows {
		out[row.Day.Format("2006-01-02")] = row.Total
	}

	return out
}

// buildSeries 补齐近 7 天，缺失日期补 0。
func buildSeries(now time.Time, counts map[string]int64) []SeriesPoint {
	out := make([]SeriesPoint, 0, 7)
	for i := 6; i >= 0; i-- {
		day := now.AddDate(0, 0, -i)
		key := day.Format("2006-01-02")
		out = append(out, SeriesPoint{Date: key[5:], Value: counts[key]})
	}

	return out
}
