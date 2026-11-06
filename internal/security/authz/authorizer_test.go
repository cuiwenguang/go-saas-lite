package authz_test

import (
	"context"
	"testing"
	"time"

	"github.com/cuiwenguang/go-saas-lite/internal/security/authz"
	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// fakeSource 用内存数据模拟 roles / user_roles，并记录调用次数验证缓存。
type fakeSource struct {
	roles map[string][]authz.Role
	users map[string][]string
	calls int
}

func (f *fakeSource) TenantRoles(ctx context.Context) ([]authz.Role, error) {
	scope, err := tenancy.ScopeFrom(ctx)
	if err != nil {
		return nil, err
	}
	f.calls++

	return f.roles[scope.TenantID], nil
}

func (f *fakeSource) UserRoleCodes(ctx context.Context, userID string) ([]string, error) {
	scope, err := tenancy.ScopeFrom(ctx)
	if err != nil {
		return nil, err
	}
	f.calls++

	return f.users[scope.TenantID+"|"+userID], nil
}

func ctxFor(tenantID, userID string) context.Context {
	return tenancy.WithScope(context.Background(), tenancy.Scope{
		TenantID: tenantID,
		UserID:   userID,
		Actor:    tenancy.ActorTenant,
	})
}

func newSource() *fakeSource {
	return &fakeSource{
		roles: map[string][]authz.Role{
			"tenant-a": {
				{Code: "admin", Perms: []authz.Perm{authz.Wildcard()}},
				{Code: "member", Perms: []authz.Perm{authz.Read("order")}},
			},
			"tenant-b": {
				{Code: "member", Perms: []authz.Perm{authz.Read("order")}},
			},
		},
		users: map[string][]string{
			"tenant-a|admin-1":  {"admin"},
			"tenant-a|member-1": {"member"},
			"tenant-b|member-1": {"member"},
		},
	}
}

func TestEnforceAllowsWildcard(t *testing.T) {
	t.Parallel()

	service := authz.NewService(newSource())
	if err := service.Enforce(ctxFor("tenant-a", "admin-1"), "order", "delete"); err != nil {
		t.Fatalf("admin should pass: %v", err)
	}
}

func TestEnforceAllowsExactPermission(t *testing.T) {
	t.Parallel()

	service := authz.NewService(newSource())
	if err := service.Enforce(ctxFor("tenant-a", "member-1"), "order", "read"); err != nil {
		t.Fatalf("member should read orders: %v", err)
	}
}

func TestEnforceDeniesMissingPermission(t *testing.T) {
	t.Parallel()

	service := authz.NewService(newSource())
	err := service.Enforce(ctxFor("tenant-a", "member-1"), "order", "update")
	if !apperr.Is(err, apperr.CodePermissionDenied) {
		t.Fatalf("want CodePermissionDenied, got %v", err)
	}
}

func TestEnforceDeniesUserWithoutRole(t *testing.T) {
	t.Parallel()

	service := authz.NewService(newSource())
	err := service.Enforce(ctxFor("tenant-a", "nobody"), "order", "read")
	if !apperr.Is(err, apperr.CodePermissionDenied) {
		t.Fatalf("want CodePermissionDenied, got %v", err)
	}
}

// 同一个 user_id 在不同租户下必须按各自角色求值。
func TestEnforceIsScopedByTenant(t *testing.T) {
	t.Parallel()

	source := newSource()
	source.users["tenant-b|admin-1"] = nil

	service := authz.NewService(source)

	if err := service.Enforce(ctxFor("tenant-a", "admin-1"), "order", "delete"); err != nil {
		t.Fatalf("tenant A admin should pass: %v", err)
	}

	err := service.Enforce(ctxFor("tenant-b", "admin-1"), "order", "delete")
	if !apperr.Is(err, apperr.CodePermissionDenied) {
		t.Fatalf("same user in tenant B must be denied, got %v", err)
	}
}

func TestEnforceUsesCacheUntilInvalidated(t *testing.T) {
	t.Parallel()

	source := newSource()
	service := authz.NewService(source)

	if err := service.Enforce(ctxFor("tenant-a", "member-1"), "order", "read"); err != nil {
		t.Fatalf("first enforce: %v", err)
	}
	callsAfterFirst := source.calls

	if err := service.Enforce(ctxFor("tenant-a", "member-1"), "order", "read"); err != nil {
		t.Fatalf("second enforce: %v", err)
	}
	if source.calls != callsAfterFirst {
		t.Fatalf("cache miss: calls %d -> %d", callsAfterFirst, source.calls)
	}

	// 权限被撤销后必须显式失效缓存。
	source.users["tenant-a|member-1"] = nil
	service.Invalidate("tenant-a")

	err := service.Enforce(ctxFor("tenant-a", "member-1"), "order", "read")
	if !apperr.Is(err, apperr.CodePermissionDenied) {
		t.Fatalf("after invalidate want CodePermissionDenied, got %v", err)
	}
}

func TestEnforceReloadsAfterTTL(t *testing.T) {
	t.Parallel()

	source := newSource()
	now := time.Now()
	service := authz.NewService(source,
		authz.WithTTL(time.Minute),
		authz.WithClock(func() time.Time { return now }),
	)

	if err := service.Enforce(ctxFor("tenant-a", "member-1"), "order", "read"); err != nil {
		t.Fatalf("first enforce: %v", err)
	}
	callsAfterFirst := source.calls

	now = now.Add(2 * time.Minute)
	source.users["tenant-a|member-1"] = nil

	err := service.Enforce(ctxFor("tenant-a", "member-1"), "order", "read")
	if !apperr.Is(err, apperr.CodePermissionDenied) {
		t.Fatalf("after ttl want CodePermissionDenied, got %v", err)
	}
	if source.calls == callsAfterFirst {
		t.Fatal("expired cache should reload from source")
	}
}

func TestHasRole(t *testing.T) {
	t.Parallel()

	service := authz.NewService(newSource())

	if !service.HasRole(ctxFor("tenant-a", "admin-1"), "admin") {
		t.Fatal("admin-1 should have admin role")
	}
	if service.HasRole(ctxFor("tenant-a", "member-1"), "admin") {
		t.Fatal("member-1 should not have admin role")
	}
}

// 权限点必须是代码常量；未登记的点不允许写入角色。
func TestRegisterValidatesPermissionFormat(t *testing.T) {
	t.Parallel()

	if err := authz.Register(authz.Read("order"), authz.Custom("order", "export")); err != nil {
		t.Fatalf("register valid perms: %v", err)
	}
	if err := authz.Register(authz.Custom("order", "list_with_filter")); err == nil {
		t.Fatal("action with underscore should be rejected")
	}
	if err := authz.Register(authz.Custom("billing:invoice", "read")); err == nil {
		t.Fatal("object containing ':' should be rejected")
	}
}
