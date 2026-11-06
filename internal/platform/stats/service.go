package stats

import (
	"context"

	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// Service 提供平台总览的聚合查询。
type Service struct {
	platform tenancy.PlatformDB
}

// NewService 创建统计服务。
func NewService(platform tenancy.PlatformDB) *Service {
	return &Service{platform: platform}
}

// Overview 返回总览数字与近 7 天新增趋势。
func (s *Service) Overview(ctx context.Context) (*OverviewOutput, error) {
	var counters struct {
		TenantTotal     int64 `gorm:"column:tenant_total"`
		TenantActive    int64 `gorm:"column:tenant_active"`
		NewThisWeek     int64 `gorm:"column:new_this_week"`
		ExpiringSoon    int64 `gorm:"column:expiring_soon"`
		ProvisionFailed int64 `gorm:"column:provision_failed"`
	}
	var usage struct {
		Total int64 `gorm:"column:total"`
	}

	series := make([]SeriesPoint, 0, 7)

	err := s.platform.Read(ctx, func(tx tenancy.Tx) error {
		if err := tx.Raw(overviewSQL).Scan(&counters); err != nil {
			return err
		}
		if err := tx.Raw(mcpCallsTotalSQL).Scan(&usage); err != nil {
			return err
		}
		return tx.Raw(newTenantsSeriesSQL).Scan(&series)
	})
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "stats overview")
	}

	return &OverviewOutput{
		TenantTotal:     counters.TenantTotal,
		TenantActive:    counters.TenantActive,
		NewThisWeek:     counters.NewThisWeek,
		ExpiringSoon:    counters.ExpiringSoon,
		ProvisionFailed: counters.ProvisionFailed,
		MCPCallsTotal:   usage.Total,
		NewSeries:       series,
	}, nil
}

// UsageTop 返回指定指标用量最高的租户（默认 MCP 调用）。
func (s *Service) UsageTop(ctx context.Context, metric string, limit int) ([]UsageTopOutput, error) {
	if metric == "" {
		metric = "mcp_calls"
	}
	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}

	items := make([]UsageTopOutput, 0, limit)
	err := s.platform.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Raw(usageTopSQL, metric, limit).Scan(&items)
	})
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "stats usage top")
	}

	return items, nil
}
