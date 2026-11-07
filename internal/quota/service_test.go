package quota_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/cuiwenguang/go-saas-lite/internal/quota"
	"github.com/cuiwenguang/go-saas-lite/internal/testutil"
	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/redisx"
)

func redisClient(t *testing.T) *redis.Client {
	t.Helper()

	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		addr = "127.0.0.1:6379"
	}

	client := redisx.New(redisx.Config{Addr: addr})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := redisx.Ping(ctx, client); err != nil {
		_ = client.Close()
		t.Skipf("redis unavailable at %s: %v", addr, err)
	}

	t.Cleanup(func() { _ = client.Close() })

	return client
}

// 用量键按租户隔离，避免测试互相干扰。
func uniqueTenant(h *testutil.Harness, t *testing.T, code string) string {
	t.Helper()

	id := h.SeedTenant(t, code)
	return id
}

func TestConsumeEnforcesPlanLimit(t *testing.T) {
	h := testutil.New(t)
	client := redisClient(t)
	service := quota.NewService(h.Platform, client)

	tenantID := uniqueTenant(h, t, "quota-limit")
	if err := h.SetPlan(t, tenantID, `{"mcp_calls":3}`); err != nil {
		t.Fatalf("set plan: %v", err)
	}

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := service.Consume(ctx, tenantID, quota.MetricMCPCalls, 1); err != nil {
			t.Fatalf("consume #%d: %v", i+1, err)
		}
	}

	err := service.Consume(ctx, tenantID, quota.MetricMCPCalls, 1)
	if !apperr.Is(err, apperr.CodeQuotaExceeded) {
		t.Fatalf("want CodeQuotaExceeded, got %v", err)
	}

	used, err := service.Used(ctx, tenantID, quota.MetricMCPCalls)
	if err != nil {
		t.Fatalf("used: %v", err)
	}
	if used != 3 {
		t.Fatalf("usage should not increase after rejection: got %d", used)
	}
}

func TestConsumeUnlimitedWhenLimitIsZero(t *testing.T) {
	h := testutil.New(t)
	client := redisClient(t)
	service := quota.NewService(h.Platform, client)

	tenantID := uniqueTenant(h, t, "quota-unlimited")
	if err := h.SetPlan(t, tenantID, `{"mcp_calls":0}`); err != nil {
		t.Fatalf("set plan: %v", err)
	}

	ctx := context.Background()
	if err := service.Consume(ctx, tenantID, quota.MetricMCPCalls, 1_000_000); err != nil {
		t.Fatalf("limit 0 means unlimited: %v", err)
	}
}

func TestFlushWritesDailyUsage(t *testing.T) {
	h := testutil.New(t)
	client := redisClient(t)
	service := quota.NewService(h.Platform, client)

	tenantID := uniqueTenant(h, t, "quota-flush")
	if err := h.SetPlan(t, tenantID, `{"mcp_tokens":1000}`); err != nil {
		t.Fatalf("set plan: %v", err)
	}

	ctx := context.Background()
	if err := service.Consume(ctx, tenantID, quota.MetricMCPToken, 42); err != nil {
		t.Fatalf("consume: %v", err)
	}

	flushed, err := service.Flush(ctx)
	if err != nil {
		t.Fatalf("flush: %v", err)
	}
	if flushed == 0 {
		t.Fatal("expected at least one flushed field")
	}

	// 落库值可查，且 Redis 已清理。
	if value := h.DailyUsage(t, tenantID, "mcp_tokens"); value != 42 {
		t.Fatalf("want 42 persisted, got %d", value)
	}

	remaining, err := client.HLen(ctx, "quota:usage").Result()
	if err != nil {
		t.Fatalf("hlen: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("usage hash should be cleared, %d fields left", remaining)
	}
}

func TestUnknownTenantIsRejected(t *testing.T) {
	h := testutil.New(t)
	client := redisClient(t)
	service := quota.NewService(h.Platform, client)

	err := service.Consume(context.Background(), uuid.Must(uuid.NewV7()).String(), quota.MetricMCPCalls, 1)
	if !apperr.Is(err, apperr.CodeTenantNotFound) {
		t.Fatalf("want CodeTenantNotFound, got %v", err)
	}
}

func TestAPICallsDailyLimit(t *testing.T) {
	h := testutil.New(t)
	client := redisClient(t)
	service := quota.NewService(h.Platform, client)

	tenantID := uniqueTenant(h, t, "quota-api-calls")
	if err := h.SetPlan(t, tenantID, `{"api_calls":2}`); err != nil {
		t.Fatalf("set plan: %v", err)
	}

	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if err := service.Consume(ctx, tenantID, quota.MetricAPICalls, 1); err != nil {
			t.Fatalf("consume #%d: %v", i+1, err)
		}
	}

	err := service.Consume(ctx, tenantID, quota.MetricAPICalls, 1)
	if !apperr.Is(err, apperr.CodeQuotaExceeded) {
		t.Fatalf("want CodeQuotaExceeded, got %v", err)
	}
}

func TestIncrMCPTokensBypassesLimit(t *testing.T) {
	h := testutil.New(t)
	client := redisClient(t)
	service := quota.NewService(h.Platform, client)

	tenantID := uniqueTenant(h, t, "quota-incr-tokens")
	if err := h.SetPlan(t, tenantID, `{"mcp_tokens":10}`); err != nil {
		t.Fatalf("set plan: %v", err)
	}

	ctx := context.Background()
	if err := service.Incr(ctx, tenantID, quota.MetricMCPToken, 50); err != nil {
		t.Fatalf("incr: %v", err)
	}

	used, err := service.Used(ctx, tenantID, quota.MetricMCPToken)
	if err != nil {
		t.Fatalf("used: %v", err)
	}
	if used != 50 {
		t.Fatalf("want 50, got %d", used)
	}

	if err := service.Check(ctx, tenantID, quota.MetricMCPToken, 1); !apperr.Is(err, apperr.CodeQuotaExceeded) {
		t.Fatalf("want CodeQuotaExceeded after overshoot, got %v", err)
	}
}

func TestCheckCountSessions(t *testing.T) {
	h := testutil.New(t)
	client := redisClient(t)
	service := quota.NewService(h.Platform, client)

	tenantID := uniqueTenant(h, t, "quota-sessions")
	if err := h.SetPlan(t, tenantID, `{"mcp_sessions":2}`); err != nil {
		t.Fatalf("set plan: %v", err)
	}

	ctx := context.Background()
	if err := service.CheckCount(ctx, tenantID, quota.MetricMCPSessions, 1); err != nil {
		t.Fatalf("1 session should be allowed: %v", err)
	}

	if err := service.CheckCount(ctx, tenantID, quota.MetricMCPSessions, 2); !apperr.Is(err, apperr.CodeQuotaExceeded) {
		t.Fatalf("want CodeQuotaExceeded at limit, got %v", err)
	}
}
