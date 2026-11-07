package file

import (
	"context"
	"errors"

	"github.com/cuiwenguang/go-saas-lite/pkg/storage"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// Purge 删除租户的全部文件对象与元数据，供归档租户清理钩子调用。
// 单个对象删除失败不阻塞其余对象，返回首个错误。
func (s *Service) Purge(ctx context.Context, tenantID string) error {
	scopeCtx := tenancy.WithScope(ctx, tenancy.Scope{
		TenantID: tenantID,
		Actor:    tenancy.ActorTenant,
	})

	var rows []File
	if err := s.db.Read(scopeCtx, func(tx tenancy.Tx) error {
		return tx.Model(&File{}).Find(&rows)
	}); err != nil {
		return err
	}

	var firstErr error
	for _, row := range rows {
		if err := s.storage.Delete(scopeCtx, row.Key); err != nil && !errors.Is(err, storage.ErrNotFound) {
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	if err := s.db.Tx(scopeCtx, func(tx tenancy.Tx) error {
		return tx.DeleteAll(&File{})
	}); err != nil {
		return err
	}

	return firstErr
}
