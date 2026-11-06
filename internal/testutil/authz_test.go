package testutil_test

import (
	"testing"

	"github.com/cuiwenguang/go-saas-lite/internal/security/authz"
	"github.com/cuiwenguang/go-saas-lite/internal/testutil"
	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
)

// 用真实 roles / user_roles 表验证求值链路（含 jsonb 解析与多租户隔离）。
func TestGormSourceEvaluatesRoles(t *testing.T) {
	h := testutil.New(t)

	tenantA := h.SeedTenant(t, "authz-a")
	tenantB := h.SeedTenant(t, "authz-b")

	adminRole := h.SeedRole(t, tenantA, "admin", []string{"*:*"})
	memberRole := h.SeedRole(t, tenantA, "member", []string{"order:read"})

	adminUser := testutil.NewUserID()
	memberUser := testutil.NewUserID()
	h.BindRole(t, tenantA, adminUser, adminRole)
	h.BindRole(t, tenantA, memberUser, memberRole)

	authorizer := authz.NewService(authz.NewGormSource(h.Tenant))

	if err := authorizer.Enforce(testutil.UserScopeContext(tenantA, adminUser), "order", "delete"); err != nil {
		t.Fatalf("admin should pass: %v", err)
	}

	if err := authorizer.Enforce(testutil.UserScopeContext(tenantA, memberUser), "order", "read"); err != nil {
		t.Fatalf("member should read order: %v", err)
	}

	err := authorizer.Enforce(testutil.UserScopeContext(tenantA, memberUser), "order", "update")
	if !apperr.Is(err, apperr.CodePermissionDenied) {
		t.Fatalf("member should not update order, got %v", err)
	}

	// 同一个用户 ID 在另一个租户下没有绑定，必须拒绝。
	err = authorizer.Enforce(testutil.UserScopeContext(tenantB, adminUser), "order", "read")
	if !apperr.Is(err, apperr.CodePermissionDenied) {
		t.Fatalf("cross-tenant evaluation must be denied, got %v", err)
	}
}

// 角色权限变更后，Invalidate 必须让新权限立即生效。
func TestInvalidatePicksUpRoleChange(t *testing.T) {
	h := testutil.New(t)

	tenant := h.SeedTenant(t, "authz-reload")
	roleID := h.SeedRole(t, tenant, "member", []string{"order:read"})
	userID := testutil.NewUserID()
	h.BindRole(t, tenant, userID, roleID)

	authorizer := authz.NewService(authz.NewGormSource(h.Tenant))
	ctx := testutil.UserScopeContext(tenant, userID)

	if err := authorizer.Enforce(ctx, "order", "read"); err != nil {
		t.Fatalf("initial read should pass: %v", err)
	}

	// 直接改库，模拟权限变更。
	h.UpdateRolePermissions(t, tenant, roleID, []string{"order:read", "order:update"})
	authorizer.Invalidate(tenant)

	if err := authorizer.Enforce(ctx, "order", "update"); err != nil {
		t.Fatalf("update should pass after role change: %v", err)
	}
}
