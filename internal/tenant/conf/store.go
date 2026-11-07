package conf

import (
	"context"
	"errors"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// Store 读写当前租户（由 ctx 中 Scope 决定）的配置。
type Store struct {
	db    tenancy.DB
	cache *configCache
}

// NewStore 创建配置存储（默认无 Redis 广播）。
func NewStore(db tenancy.DB) *Store {
	return &Store{db: db, cache: newConfigCache(nil)}
}

// WithRedis 启用跨实例缓存失效广播。
func (s *Store) WithRedis(client *redis.Client) *Store {
	s.cache = newConfigCache(client)
	return s
}

// Get 读取单个配置；命中本地缓存时直接返回。
func (s *Store) Get(ctx context.Context, namespace, key string) (string, error) {
	scope, err := tenancy.ScopeFrom(ctx)
	if err != nil {
		return "", err
	}
	if value, ok := s.cache.get(scope.TenantID, namespace, key); ok {
		return value, nil
	}

	var row Config
	err = s.db.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Where("namespace = ? AND key = ?", namespace, key).First(&row)
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", apperr.New(apperr.CodeNotFound, "config not found")
	}
	if err != nil {
		return "", apperr.Wrap(err, apperr.CodeInternal, "get config")
	}

	s.cache.set(scope.TenantID, namespace, key, row.Value)

	return row.Value, nil
}

// List 列出某个 namespace 下的全部配置。
func (s *Store) List(ctx context.Context, namespace string) ([]Entry, error) {
	var rows []Config
	err := s.db.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Where("namespace = ?", namespace).Order("key ASC").Find(&rows)
	})
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "list configs")
	}

	entries := make([]Entry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, Entry{Namespace: row.Namespace, Key: row.Key, Value: row.Value, UpdatedAt: row.UpdatedAt})
	}

	return entries, nil
}

// Set 写入或更新配置。
func (s *Store) Set(ctx context.Context, namespace, key, value string) error {
	if namespace == "" || key == "" {
		return apperr.New(apperr.CodeInvalidArgument, "namespace and key are required")
	}

	err := s.db.Tx(ctx, func(tx tenancy.Tx) error {
		var existing Config
		findErr := tx.Where("namespace = ? AND key = ?", namespace, key).First(&existing)
		switch {
		case findErr == nil:
			return tx.Where("namespace = ? AND key = ?", namespace, key).
				Update(&Config{}, map[string]any{"value": value})
		case errors.Is(findErr, gorm.ErrRecordNotFound):
			return tx.Create(&Config{Namespace: namespace, Key: key, Value: value})
		default:
			return findErr
		}
	})
	if err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "set config")
	}

	if scope, scopeErr := tenancy.ScopeFrom(ctx); scopeErr == nil {
		s.cache.set(scope.TenantID, namespace, key, value)
		s.cache.publish(ctx, scope.TenantID, namespace, key)
	}

	return nil
}

// Delete 删除配置。
func (s *Store) Delete(ctx context.Context, namespace, key string) error {
	err := s.db.Tx(ctx, func(tx tenancy.Tx) error {
		return tx.Where("namespace = ? AND key = ?", namespace, key).Delete(&Config{})
	})
	if err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "delete config")
	}

	if scope, scopeErr := tenancy.ScopeFrom(ctx); scopeErr == nil {
		s.cache.delete(scope.TenantID, namespace, key)
		s.cache.publish(ctx, scope.TenantID, namespace, key)
	}

	return nil
}
