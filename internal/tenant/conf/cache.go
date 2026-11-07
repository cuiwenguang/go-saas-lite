package conf

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	configCacheTTL  = 30 * time.Second
	configReloadKey = "tenantconf:reload"
)

type cacheEntry struct {
	value   string
	expires time.Time
}

type reloadMessage struct {
	TenantID  string `json:"tenant_id"`
	Namespace string `json:"namespace"`
	Key       string `json:"key"`
}

// configCache 是进程内配置缓存；Redis 只用于跨实例失效广播。
type configCache struct {
	client  *redis.Client
	ttl     time.Duration
	mu      sync.RWMutex
	entries map[string]cacheEntry
}

func newConfigCache(client *redis.Client) *configCache {
	return &configCache{
		client:  client,
		ttl:     configCacheTTL,
		entries: make(map[string]cacheEntry),
	}
}

func cacheKey(tenantID, namespace, key string) string {
	return tenantID + "|" + namespace + "|" + key
}

func (c *configCache) get(tenantID, namespace, key string) (string, bool) {
	c.mu.RLock()
	entry, ok := c.entries[cacheKey(tenantID, namespace, key)]
	c.mu.RUnlock()
	if !ok || time.Now().After(entry.expires) {
		return "", false
	}

	return entry.value, true
}

func (c *configCache) set(tenantID, namespace, key, value string) {
	c.mu.Lock()
	c.entries[cacheKey(tenantID, namespace, key)] = cacheEntry{value: value, expires: time.Now().Add(c.ttl)}
	c.mu.Unlock()
}

func (c *configCache) delete(tenantID, namespace, key string) {
	c.mu.Lock()
	delete(c.entries, cacheKey(tenantID, namespace, key))
	c.mu.Unlock()
}

func (c *configCache) publish(ctx context.Context, tenantID, namespace, key string) {
	if c.client == nil {
		return
	}

	payload, err := json.Marshal(reloadMessage{TenantID: tenantID, Namespace: namespace, Key: key})
	if err != nil {
		return
	}

	_ = c.client.Publish(ctx, configReloadKey, payload).Err()
}

func (c *configCache) subscribe(ctx context.Context) {
	if c.client == nil {
		return
	}

	sub := c.client.Subscribe(ctx, configReloadKey)
	defer func() { _ = sub.Close() }()

	for message := range sub.Channel() {
		var payload reloadMessage
		if err := json.Unmarshal([]byte(message.Payload), &payload); err != nil {
			continue
		}
		c.delete(payload.TenantID, payload.Namespace, payload.Key)
	}
}
