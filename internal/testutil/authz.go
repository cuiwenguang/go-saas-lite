package testutil

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// SeedRole 在指定租户下插入角色，返回角色 ID。
func (h *Harness) SeedRole(t *testing.T, tenantID, code string, permissions []string) string {
	t.Helper()

	id := uuid.Must(uuid.NewV7()).String()
	raw, err := json.Marshal(permissions)
	if err != nil {
		t.Fatalf("marshal permissions: %v", err)
	}

	err = h.Tenant.Tx(Context(tenantID), func(tx tenancy.Tx) error {
		return tx.Create(&Role{
			ID:          id,
			Code:        code,
			Name:        code,
			Permissions: string(raw),
			IsBuiltin:   true,
		})
	})
	if err != nil {
		t.Fatalf("seed role %s: %v", code, err)
	}

	return id
}

// BindRole 把角色绑定到用户。
func (h *Harness) BindRole(t *testing.T, tenantID, userID, roleID string) {
	t.Helper()

	err := h.Tenant.Tx(Context(tenantID), func(tx tenancy.Tx) error {
		return tx.Create(&UserRole{UserID: userID, RoleID: roleID})
	})
	if err != nil {
		t.Fatalf("bind role: %v", err)
	}
}

// NewUserID 生成一个测试用用户 ID。
func NewUserID() string {
	return uuid.Must(uuid.NewV7()).String()
}

// UpdateRolePermissions 直接改库里的权限集合，模拟权限变更。
func (h *Harness) UpdateRolePermissions(t *testing.T, tenantID, roleID string, permissions []string) {
	t.Helper()

	raw, err := json.Marshal(permissions)
	if err != nil {
		t.Fatalf("marshal permissions: %v", err)
	}

	err = h.Tenant.Tx(Context(tenantID), func(tx tenancy.Tx) error {
		return tx.Where("id = ?", roleID).Update(&Role{}, map[string]any{"permissions": string(raw)})
	})
	if err != nil {
		t.Fatalf("update role permissions: %v", err)
	}
}

// UserScopeContext 构造带用户身份的 ctx，供权限求值使用。
func UserScopeContext(tenantID, userID string) context.Context {
	return tenancy.WithScope(context.Background(), tenancy.Scope{
		TenantID: tenantID,
		UserID:   userID,
		Actor:    tenancy.ActorTenant,
	})
}
