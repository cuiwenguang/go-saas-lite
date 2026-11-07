package members_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/cuiwenguang/go-saas-lite/internal/quota"
	"github.com/cuiwenguang/go-saas-lite/internal/tenant/members"
	"github.com/cuiwenguang/go-saas-lite/internal/testutil"
	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/redisx"
)

func testRedis(t *testing.T) *redis.Client {
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

func TestCreateEnforcesMaxUsers(t *testing.T) {
	h := testutil.New(t)
	quotaService := quota.NewService(h.Platform, testRedis(t))

	tenantID := h.SeedTenant(t, "members-limit")
	if err := h.SetPlan(t, tenantID, `{"max_users":1}`); err != nil {
		t.Fatalf("set plan: %v", err)
	}
	_ = h.SeedRole(t, tenantID, "member", nil)

	service := members.NewService(h.Tenant, h.Identity(), quotaService, nil)
	ctx := testutil.Context(tenantID)
	unique := strings.ReplaceAll(uuid.Must(uuid.NewV7()).String(), "-", "")

	if _, err := service.Create(ctx, members.CreateInput{Email: "one-" + unique + "@members.test"}); err != nil {
		t.Fatalf("first member should be allowed: %v", err)
	}

	_, err := service.Create(ctx, members.CreateInput{Email: "two-" + unique + "@members.test"})
	if !apperr.Is(err, apperr.CodeQuotaExceeded) {
		t.Fatalf("want CodeQuotaExceeded, got %v", err)
	}
}
