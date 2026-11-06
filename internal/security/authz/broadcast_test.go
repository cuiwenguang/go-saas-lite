package authz_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/cuiwenguang/go-saas-lite/internal/security/authz"
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

func TestRedisBroadcasterRoundTrip(t *testing.T) {
	client := redisClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	publisher := authz.NewRedisBroadcaster(client)
	subscriber := authz.NewRedisBroadcaster(client)

	received := make(chan string, 1)
	go func() {
		_ = subscriber.Subscribe(ctx, func(tenantID string) { received <- tenantID })
	}()

	// 订阅建立是异步的，反复发布直到收到。
	deadline := time.After(4 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case tenantID := <-received:
			if tenantID != "tenant-broadcast" {
				t.Fatalf("unexpected tenant id %q", tenantID)
			}
			return
		case <-ticker.C:
			_ = publisher.Publish(ctx, "tenant-broadcast")
		case <-deadline:
			t.Fatal("no message received from redis pub/sub")
		}
	}
}

// 其他副本广播失效后，本副本的进程内缓存必须被清掉并重新求值。
func TestWatchInvalidationClearsCache(t *testing.T) {
	client := redisClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	source := newSource()
	service := authz.NewService(source, authz.WithTTL(time.Hour)).
		WithBroadcaster(authz.NewRedisBroadcaster(client))

	tenantID := "tenant-a"
	userID := "member-1"

	if err := service.Enforce(ctxFor(tenantID, userID), "order", "read"); err != nil {
		t.Fatalf("prime cache: %v", err)
	}

	// 另一个副本改了权限并广播。
	source.users[tenantID+"|"+userID] = nil

	go func() { _ = service.WatchInvalidation(ctx) }()

	other := authz.NewRedisBroadcaster(client)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-ticker.C:
			_ = other.Publish(ctx, tenantID)

			before := source.calls
			_ = service.Enforce(ctxFor(tenantID, userID), "order", "read")
			if source.calls > before {
				// 缓存已被清掉并重新加载。
				if err := service.Enforce(ctxFor(tenantID, userID), "order", "read"); err == nil {
					t.Fatal("permission should be revoked after invalidation")
				}
				return
			}
		case <-deadline:
			t.Fatal("invalidation broadcast was not observed")
		}
	}
}
