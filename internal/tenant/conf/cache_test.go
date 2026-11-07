package conf

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/cuiwenguang/go-saas-lite/internal/testutil"
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

func TestCacheInvalidationAcrossStores(t *testing.T) {
	h := testutil.New(t)
	client := testRedis(t)
	tenantID := h.SeedTenant(t, "conf-cache")
	ctx := testutil.Context(tenantID)

	storeA := NewStore(h.Tenant).WithRedis(client)
	storeB := NewStore(h.Tenant).WithRedis(client)

	subCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go storeA.cache.subscribe(subCtx)
	go storeB.cache.subscribe(subCtx)
	time.Sleep(100 * time.Millisecond)

	if err := storeA.Set(ctx, "general", "site_name", "old"); err != nil {
		t.Fatalf("set old: %v", err)
	}
	if value, err := storeB.Get(ctx, "general", "site_name"); err != nil || value != "old" {
		t.Fatalf("warm cache: %q %v", value, err)
	}

	if err := storeA.Set(ctx, "general", "site_name", "new"); err != nil {
		t.Fatalf("set new: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		value, err := storeB.Get(ctx, "general", "site_name")
		if err != nil {
			t.Fatalf("get after invalidate: %v", err)
		}
		if value == "new" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatal("store B did not see invalidated value")
}
