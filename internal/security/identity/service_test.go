package identity_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cuiwenguang/go-saas-lite/internal/platform/audit"
	"github.com/cuiwenguang/go-saas-lite/internal/platform/tenants"
	"github.com/cuiwenguang/go-saas-lite/internal/security/identity"
	"github.com/cuiwenguang/go-saas-lite/internal/testutil"
	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

func uniqueEmail(prefix string) string {
	hex := strings.ReplaceAll(uuid.Must(uuid.NewV7()).String(), "-", "")

	return prefix + "-" + hex[len(hex)-12:] + "@identity.test"
}

func newTenant(t *testing.T) (*testutil.Harness, string, string) {
	t.Helper()

	h := testutil.New(t)
	service := tenants.NewService(h.Platform, h.Tenant, audit.NewService(h.Platform), h.Identity())
	hex := strings.ReplaceAll(uuid.Must(uuid.NewV7()).String(), "-", "")
	email := uniqueEmail("admin")

	created, _, err := service.Create(t.Context(), tenants.CreateInput{
		Code:          "id-" + hex[len(hex)-10:],
		Name:          "Identity Tenant",
		Timezone:      "Asia/Shanghai",
		Locale:        "zh-CN",
		AdminEmail:    email,
		AdminPassword: "password123",
	})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	return h, created.ID, email
}

// 开通租户时管理员邮箱要被登记，登录侧才能凭邮箱定位租户。
func TestTenantProvisionIndexesAdminEmail(t *testing.T) {
	h, tenantID, email := newTenant(t)

	hit, err := h.Identity().Lookup(t.Context(), identity.TypeEmail, strings.ToUpper(email))
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if hit.TenantID != tenantID {
		t.Fatalf("tenant id: want %s, got %s", tenantID, hit.TenantID)
	}
	if hit.Identifier != email {
		t.Fatalf("identifier should be normalized to lower case, got %s", hit.Identifier)
	}
}

// 登记是幂等的；同一邮箱指向别的用户要拒绝。
func TestEnsureIdempotentAndConflict(t *testing.T) {
	h, tenantID, email := newTenant(t)
	svc := h.Identity()

	var userID string
	scopeCtx := tenancy.WithScope(t.Context(), tenancy.Scope{TenantID: tenantID, Actor: tenancy.ActorTenant})
	var users []testutil.User
	if err := h.Tenant.Read(scopeCtx, func(tx tenancy.Tx) error { return tx.Find(&users) }); err != nil {
		t.Fatalf("load users: %v", err)
	}
	if len(users) == 0 {
		t.Fatal("tenant has no users")
	}
	userID = users[0].ID

	if err := svc.Ensure(t.Context(), tenantID, userID, []identity.Identifier{{Type: identity.TypeEmail, Value: email}}); err != nil {
		t.Fatalf("re-ensure should be idempotent: %v", err)
	}

	err := svc.Ensure(t.Context(), tenantID, uuid.Must(uuid.NewV7()).String(),
		[]identity.Identifier{{Type: identity.TypeEmail, Value: email}})
	if err == nil {
		t.Fatal("conflicting identity should be rejected")
	}
	if appErr := apperr.From(err); appErr == nil || appErr.Code != apperr.CodeConflict {
		t.Fatalf("want conflict, got %v", err)
	}
}

// 删除用户的索引后，对账要能补回来。
func TestRepairRestoresMissingIdentity(t *testing.T) {
	h, tenantID, email := newTenant(t)
	svc := h.Identity()

	hit, err := svc.Lookup(t.Context(), identity.TypeEmail, email)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if err := svc.Remove(t.Context(), tenantID, hit.UserID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := svc.Lookup(t.Context(), identity.TypeEmail, email); err == nil {
		t.Fatal("identity should be gone before repair")
	}

	if _, err := svc.Repair(t.Context(), 200); err != nil {
		t.Fatalf("repair: %v", err)
	}

	restored, err := svc.Lookup(t.Context(), identity.TypeEmail, email)
	if err != nil {
		t.Fatalf("identity not restored: %v", err)
	}
	if restored.UserID != hit.UserID {
		t.Fatalf("user id mismatch after repair: %s vs %s", restored.UserID, hit.UserID)
	}
}
