package conf

import (
	"context"

	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// Service 是租户配置模块的唯一对外入口。
type Service struct {
	store *Store
}

// NewService 创建配置服务。
func NewService(db tenancy.DB) *Service {
	return &Service{store: NewStore(db)}
}

// Get 读取单个配置。
func (s *Service) Get(ctx context.Context, namespace, key string) (string, error) {
	return s.store.Get(ctx, namespace, key)
}

// List 列出某个 namespace 下的全部配置。
func (s *Service) List(ctx context.Context, namespace string) ([]Entry, error) {
	return s.store.List(ctx, namespace)
}

// Set 写入或更新配置。
func (s *Service) Set(ctx context.Context, namespace, key, value string) error {
	return s.store.Set(ctx, namespace, key, value)
}

// Delete 删除配置。
func (s *Service) Delete(ctx context.Context, namespace, key string) error {
	return s.store.Delete(ctx, namespace, key)
}
