package authz

import (
	"context"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"
)

const reloadChannelPrefix = "authz:reload:"

// Broadcaster 把失效事件广播给其他副本。
type Broadcaster interface {
	// Publish 通知其他副本丢弃指定租户的缓存。
	Publish(ctx context.Context, tenantID string) error
	// Subscribe 阻塞订阅，收到事件后回调；ctx 取消时返回。
	Subscribe(ctx context.Context, handler func(tenantID string)) error
}

// RedisBroadcaster 基于 redis pub/sub 实现跨副本失效。
type RedisBroadcaster struct {
	client redis.UniversalClient
}

// NewRedisBroadcaster 创建广播器。
func NewRedisBroadcaster(client redis.UniversalClient) Broadcaster {
	return &RedisBroadcaster{client: client}
}

func (b *RedisBroadcaster) Publish(ctx context.Context, tenantID string) error {
	if err := b.client.Publish(ctx, reloadChannelPrefix+tenantID, "1").Err(); err != nil {
		return fmt.Errorf("authz: publish reload for tenant %s: %w", tenantID, err)
	}
	return nil
}

func (b *RedisBroadcaster) Subscribe(ctx context.Context, handler func(tenantID string)) error {
	pubsub := b.client.PSubscribe(ctx, reloadChannelPrefix+"*")
	defer func() { _ = pubsub.Close() }()

	if _, err := pubsub.Receive(ctx); err != nil {
		return fmt.Errorf("authz: subscribe reload channel: %w", err)
	}

	channel := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return nil
		case message, ok := <-channel:
			if !ok {
				return nil
			}
			if tenantID := strings.TrimPrefix(message.Channel, reloadChannelPrefix); tenantID != "" {
				handler(tenantID)
			}
		}
	}
}

// WithBroadcaster 注入跨副本广播器。
func (s *Service) WithBroadcaster(broadcaster Broadcaster) *Service {
	s.broadcaster = broadcaster
	return s
}

// InvalidateTenant 清本地缓存并广播给其他副本。
// 权限或绑定变更后调用；广播失败不影响本地失效。
func (s *Service) InvalidateTenant(ctx context.Context, tenantID string) error {
	s.Invalidate(tenantID)

	if s.broadcaster == nil {
		return nil
	}

	return s.broadcaster.Publish(ctx, tenantID)
}

// WatchInvalidation 订阅其他副本的失效广播并清本地缓存。
// 阻塞运行，ctx 取消时返回；应在后台 goroutine 中启动。
func (s *Service) WatchInvalidation(ctx context.Context) error {
	if s.broadcaster == nil {
		return nil
	}

	return s.broadcaster.Subscribe(ctx, s.Invalidate)
}
