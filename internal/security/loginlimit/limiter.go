// Package loginlimit 用 Redis 计数实现密码登录失败锁定。
package loginlimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	defaultMax    int64 = 5
	defaultWindow       = 15 * time.Minute
)

// Limiter 是固定窗口的登录失败计数器。
type Limiter struct {
	client *redis.Client
	max    int64
	window time.Duration
}

// New 创建限流器；client 为 nil 时不生效。
func New(client *redis.Client) *Limiter {
	return &Limiter{client: client, max: defaultMax, window: defaultWindow}
}

// Key 生成登录失败计数键。
func Key(kind, identifier string) string {
	return fmt.Sprintf("login:fail:%s:%s", kind, identifier)
}

// Allow 判断账号是否未被锁定。
func (l *Limiter) Allow(ctx context.Context, key string) bool {
	if l == nil || l.client == nil {
		return true
	}

	count, err := l.client.Get(ctx, key).Int64()
	if err != nil {
		return true
	}

	return count < l.max
}

// Fail 记录一次失败。
func (l *Limiter) Fail(ctx context.Context, key string) {
	if l == nil || l.client == nil {
		return
	}

	count, err := l.client.Incr(ctx, key).Result()
	if err != nil {
		return
	}
	if count == 1 {
		_ = l.client.Expire(ctx, key, l.window).Err()
	}
}

// Reset 清除失败计数。
func (l *Limiter) Reset(ctx context.Context, key string) {
	if l == nil || l.client == nil {
		return
	}

	_ = l.client.Del(ctx, key).Err()
}
