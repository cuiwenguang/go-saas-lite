package tenant

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cuiwenguang/go-saas-lite/internal/security/authz"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// SyncBuiltinRoles 把内置角色权限覆写成代码常量（readme 2.3）。
// 每次发版执行一次，保证老租户拿到新增权限点。
func SyncBuiltinRoles(ctx context.Context, platform tenancy.PlatformDB, tenantDB tenancy.DB) (int, error) {
	var tenants []Tenant
	err := platform.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Where("deleted_at IS NULL").Find(&tenants)
	})
	if err != nil {
		return 0, fmt.Errorf("list tenants: %w", err)
	}

	synced := 0
	for _, item := range tenants {
		scopeCtx := tenancy.WithScope(ctx, tenancy.Scope{
			TenantID: item.ID,
			Actor:    tenancy.ActorTenant,
		})

		if syncErr := tenantDB.Tx(scopeCtx, func(tx tenancy.Tx) error {
			for code, perms := range authz.BuiltinRoles() {
				roleID, ensureErr := ensureRole(tx, code, perms)
				if ensureErr != nil {
					return ensureErr
				}

				items := make([]string, 0, len(perms))
				for _, perm := range perms {
					items = append(items, perm.String())
				}
				raw, marshalErr := json.Marshal(items)
				if marshalErr != nil {
					return marshalErr
				}

				if updateErr := tx.Where("id = ?", roleID).
					Update(&Role{}, map[string]any{"permissions": string(raw)}); updateErr != nil {
					return updateErr
				}
			}
			return nil
		}); syncErr != nil {
			return synced, fmt.Errorf("sync tenant %s: %w", item.ID, syncErr)
		}
		synced++
	}

	return synced, nil
}
