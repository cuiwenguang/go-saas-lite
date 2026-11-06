// Package redisx 提供 Redis 客户端与键规范。
//
// key 统一 tenant:{tid}:xxx 前缀，由框架拼接，业务不自拼（readme 必查清单 4）。
package redisx

import (
	"context"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"
)

// Config 是 Redis 连接参数。
type Config struct {
	Addr     string
	Password string
	DB       int
}

// New 创建客户端。
func New(cfg Config) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
}

// Ping 校验连通性，供 /readyz 使用。
func Ping(ctx context.Context, client redis.UniversalClient) error {
	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis: %w", err)
	}
	return nil
}

// TenantKey 拼接租户维度的 key：tenant:{tid}:scope...
func TenantKey(tenantID string, parts ...string) string {
	segments := make([]string, 0, len(parts)+2)
	segments = append(segments, "tenant", tenantID)
	segments = append(segments, parts...)

	return strings.Join(segments, ":")
}
