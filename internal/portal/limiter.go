package portal

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type memEntry struct {
	count   int64
	expires time.Time
}

// limiter 是按 key 的固定窗口限流器；Redis 不可用时退化为进程内。
type limiter struct {
	client *redis.Client

	mu  sync.Mutex
	mem map[string]memEntry
}

func newLimiter(client *redis.Client) *limiter {
	return &limiter{client: client, mem: make(map[string]memEntry)}
}

func (l *limiter) Allow(ctx context.Context, key string, limit int, window time.Duration) bool {
	if limit <= 0 {
		return true
	}

	if l.client != nil {
		return l.allowRedis(ctx, key, limit, window)
	}

	return l.allowMemory(key, limit, window)
}

func (l *limiter) allowRedis(ctx context.Context, key string, limit int, window time.Duration) bool {
	bucket := fmt.Sprintf("portal:rate:%s:%d", key, time.Now().Unix()/int64(window.Seconds()))
	count, err := l.client.Incr(ctx, bucket).Result()
	if err != nil {
		return true
	}
	if count == 1 {
		_ = l.client.Expire(ctx, bucket, window).Err()
	}

	return count <= int64(limit)
}

func (l *limiter) allowMemory(key string, limit int, window time.Duration) bool {
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	entry, ok := l.mem[key]
	if !ok || now.After(entry.expires) {
		l.mem[key] = memEntry{count: 1, expires: now.Add(window)}
		return true
	}

	entry.count++
	l.mem[key] = entry

	return entry.count <= int64(limit)
}
