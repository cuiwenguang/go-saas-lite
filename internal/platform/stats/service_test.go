package stats_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cuiwenguang/go-saas-lite/internal/platform/stats"
	"github.com/cuiwenguang/go-saas-lite/internal/platform/tenants"
	"github.com/cuiwenguang/go-saas-lite/internal/quota"
	"github.com/cuiwenguang/go-saas-lite/internal/testutil"
)

func TestOverviewAndUsageTop(t *testing.T) {
	h := testutil.New(t)
	ctx := context.Background()
	service := stats.NewService(h.Platform)

	tenantService := tenants.NewService(h.Platform, h.Tenant, nil, h.Identity())
	created, _, err := tenantService.Create(ctx, tenants.CreateInput{
		Code:          "stats-" + strings.ReplaceAll(uuid.Must(uuid.NewV7()).String(), "-", "")[:10],
		Name:          "Stats Tenant",
		Timezone:      "Asia/Shanghai",
		Locale:        "zh-CN",
		AdminEmail:    "admin@stats.test",
		AdminPassword: "password123",
	})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	// 直接写一条用量快照，验证总量与排行口径；用递增基数保证本轮排名第一。
	seededCalls := 1_000_000_000 + time.Now().UnixNano()%1_000_000_000
	if err := h.PlatformGorm.Exec(
		`INSERT INTO usage_daily (tenant_id, metric, day, value) VALUES (?, ?, current_date, ?)`,
		created.ID, string(quota.MetricMCPCalls), seededCalls,
	).Error; err != nil {
		t.Fatalf("seed usage: %v", err)
	}

	overview, err := service.Overview(ctx)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if overview.TenantTotal < 1 || overview.TenantActive < 1 {
		t.Fatalf("unexpected overview counters: %+v", overview)
	}
	if len(overview.NewSeries) != 7 {
		t.Fatalf("want 7 series points, got %d", len(overview.NewSeries))
	}
	if overview.MCPCallsTotal < seededCalls {
		t.Fatalf("mcp calls total should include seeded usage, got %d", overview.MCPCallsTotal)
	}

	top, err := service.UsageTop(ctx, string(quota.MetricMCPCalls), 20)
	if err != nil {
		t.Fatalf("usage top: %v", err)
	}
	ranked := false
	for _, item := range top {
		if item.TenantID == created.ID && item.Value >= seededCalls {
			ranked = true
		}
	}
	if !ranked {
		t.Fatalf("seeded tenant missing from usage top: %+v", top)
	}
}
