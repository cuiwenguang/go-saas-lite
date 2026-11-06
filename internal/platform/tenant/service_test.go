package tenant_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/cuiwenguang/go-saas-lite/internal/platform/tenant"
	"github.com/cuiwenguang/go-saas-lite/internal/testutil"
	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

func uniqueCode(prefix string) string {
	hex := strings.ReplaceAll(uuid.Must(uuid.NewV7()).String(), "-", "")
	return prefix + "-" + hex[len(hex)-10:]
}

func newService(t *testing.T) (*tenant.Service, *testutil.Harness) {
	t.Helper()

	h := testutil.New(t)
	return tenant.NewService(h.Platform, h.Tenant), h
}

func createInput() tenant.CreateInput {
	return tenant.CreateInput{
		Code:             uniqueCode("acme"),
		Name:             "Acme",
		Timezone:         "Asia/Shanghai",
		Locale:           "zh-CN",
		ContactEmail:     "it@acme.test",
		AdminEmail:       "admin@acme.test",
		AdminDisplayName: "管理员",
	}
}

// 开通流水线要一次建好租户、内置角色、管理员与绑定。
func TestCreateProvisionsTenant(t *testing.T) {
	service, h := newService(t)

	created, password, err := service.Create(context.Background(), createInput())
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	if created.Status != tenant.StatusActive {
		t.Fatalf("tenant status: want %s, got %s", tenant.StatusActive, created.Status)
	}
	if len(password) < 16 {
		t.Fatalf("generated password looks too short: %q", password)
	}

	scopeCtx := testutil.Context(created.ID)

	var roles []testutil.Role
	if err := h.Tenant.Read(scopeCtx, func(tx tenancy.Tx) error { return tx.Find(&roles) }); err != nil {
		t.Fatalf("load roles: %v", err)
	}
	if len(roles) != 2 {
		t.Fatalf("want 2 builtin roles, got %d", len(roles))
	}

	var users []testutil.User
	if err := h.Tenant.Read(scopeCtx, func(tx tenancy.Tx) error { return tx.Find(&users) }); err != nil {
		t.Fatalf("load users: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("want 1 admin user, got %d", len(users))
	}
	if !users[0].MustChangePassword {
		t.Fatal("initial admin must change password on first login")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(users[0].PasswordHash), []byte(password)); err != nil {
		t.Fatalf("stored hash does not match returned password: %v", err)
	}

	var bindings []testutil.UserRole
	if err := h.Tenant.Read(scopeCtx, func(tx tenancy.Tx) error { return tx.Find(&bindings) }); err != nil {
		t.Fatalf("load bindings: %v", err)
	}
	if len(bindings) != 1 {
		t.Fatalf("want 1 admin binding, got %d", len(bindings))
	}
}

func TestCreateRejectsDuplicateCode(t *testing.T) {
	service, _ := newService(t)

	input := createInput()
	if _, _, err := service.Create(context.Background(), input); err != nil {
		t.Fatalf("first create: %v", err)
	}

	input.AdminEmail = "other@acme.test"
	_, _, err := service.Create(context.Background(), input)
	if !apperr.Is(err, apperr.CodeInvalidArgument) && !apperr.Is(err, apperr.CodeConflict) {
		t.Fatalf("want conflict for duplicate code, got %v", err)
	}
}

func TestStatusTransitionsAndArchive(t *testing.T) {
	service, _ := newService(t)
	ctx := context.Background()

	created, _, err := service.Create(ctx, createInput())
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if _, err := service.UpdateStatus(ctx, created.ID, tenant.StatusSuspended); err != nil {
		t.Fatalf("active -> suspended should be allowed: %v", err)
	}
	if _, err := service.UpdateStatus(ctx, created.ID, tenant.StatusActive); err != nil {
		t.Fatalf("suspended -> active should be allowed: %v", err)
	}
	if _, err := service.UpdateStatus(ctx, created.ID, tenant.StatusPending); !apperr.Is(err, apperr.CodeInvalidArgument) {
		t.Fatalf("active -> pending must be rejected, got %v", err)
	}

	if err := service.Archive(ctx, created.ID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if _, err := service.UpdateStatus(ctx, created.ID, tenant.StatusActive); !apperr.Is(err, apperr.CodeInvalidArgument) {
		t.Fatalf("archived tenant must not be reactivated, got %v", err)
	}
}

func TestGetMissingTenant(t *testing.T) {
	service, _ := newService(t)

	_, err := service.Get(context.Background(), uuid.Must(uuid.NewV7()).String())
	if !apperr.Is(err, apperr.CodeTenantNotFound) {
		t.Fatalf("want CodeTenantNotFound, got %v", err)
	}
}

func TestListFiltersByStatus(t *testing.T) {
	service, _ := newService(t)
	ctx := context.Background()

	created, _, err := service.Create(ctx, createInput())
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	items, total, err := service.List(ctx, tenant.ListFilter{Status: tenant.StatusActive, Page: 1, Size: 50})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total == 0 || len(items) == 0 {
		t.Fatal("expected at least one active tenant")
	}

	found := false
	for _, item := range items {
		if item.ID == created.ID {
			found = true
		}
		if item.Status != tenant.StatusActive {
			t.Fatalf("filter leaked status %s", item.Status)
		}
	}
	if !found {
		t.Fatal("created tenant missing from active list")
	}
}
