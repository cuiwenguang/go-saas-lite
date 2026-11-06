package tenants_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/cuiwenguang/go-saas-lite/internal/platform/audit"
	"github.com/cuiwenguang/go-saas-lite/internal/platform/tenants"
	"github.com/cuiwenguang/go-saas-lite/internal/testutil"
	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

func uniqueCode(prefix string) string {
	hex := strings.ReplaceAll(uuid.Must(uuid.NewV7()).String(), "-", "")
	return prefix + "-" + hex[len(hex)-10:]
}

func newService(t *testing.T) (*tenants.Service, *testutil.Harness) {
	t.Helper()

	h := testutil.New(t)
	return tenants.NewService(h.Platform, h.Tenant, audit.NewService(h.Platform), h.Identity()), h
}

func createInput() tenants.CreateInput {
	suffix := strings.ReplaceAll(uuid.Must(uuid.NewV7()).String(), "-", "")[8:]

	return tenants.CreateInput{
		Code:             uniqueCode("acme"),
		Name:             "Acme",
		Timezone:         "Asia/Shanghai",
		Locale:           "zh-CN",
		ContactEmail:     "it@acme.test",
		AdminEmail:       "admin-" + suffix + "@acme.test", // 邮箱全局唯一
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

	if created.Status != tenants.StatusActive {
		t.Fatalf("tenant status: want %s, got %s", tenants.StatusActive, created.Status)
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

	if _, err := service.UpdateStatus(ctx, created.ID, tenants.StatusSuspended); err != nil {
		t.Fatalf("active -> suspended should be allowed: %v", err)
	}
	if _, err := service.UpdateStatus(ctx, created.ID, tenants.StatusActive); err != nil {
		t.Fatalf("suspended -> active should be allowed: %v", err)
	}
	if _, err := service.UpdateStatus(ctx, created.ID, tenants.StatusPending); !apperr.Is(err, apperr.CodeInvalidArgument) {
		t.Fatalf("active -> pending must be rejected, got %v", err)
	}

	if err := service.Archive(ctx, created.ID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if _, err := service.UpdateStatus(ctx, created.ID, tenants.StatusActive); !apperr.Is(err, apperr.CodeInvalidArgument) {
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

	items, total, err := service.List(ctx, tenants.ListFilter{Status: tenants.StatusActive, Page: 1, Size: 50})
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
		if item.Status != tenants.StatusActive {
			t.Fatalf("filter leaked status %s", item.Status)
		}
	}
	if !found {
		t.Fatal("created tenant missing from active list")
	}
}

// 详情页「配额与用量」：限额来自套餐，用量来自 usage_daily，趋势补满 7 天。
func TestUsageReportsQuotaAndMetrics(t *testing.T) {
	service, h := newService(t)
	ctx := context.Background()

	created, _, err := service.Create(ctx, createInput())
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	if err := h.PlatformGorm.Exec(
		`INSERT INTO usage_daily (tenant_id, metric, day, value) VALUES (?, 'mcp_calls', current_date, 500)`,
		created.ID,
	).Error; err != nil {
		t.Fatalf("seed usage: %v", err)
	}

	out, err := service.Usage(ctx, created.ID)
	if err != nil {
		t.Fatalf("tenant usage: %v", err)
	}
	if out.PlanCode != "free" {
		t.Fatalf("plan code: want free, got %q", out.PlanCode)
	}
	if out.MaxUsers == 0 {
		t.Fatal("free plan should carry a user cap")
	}
	if out.Usage.Users != 1 {
		t.Fatalf("want 1 tenant user, got %d", out.Usage.Users)
	}
	if out.Usage.MCPCalls != 500 {
		t.Fatalf("want 500 mcp calls, got %d", out.Usage.MCPCalls)
	}
	if len(out.Series) != 7 {
		t.Fatalf("want 7 series points, got %d", len(out.Series))
	}
}

// 详情页依赖的聚合能力：状态计数、改字段/续费、用户列表、重置密码、重试守卫。
func TestSummaryUpdateUsersAndCredentials(t *testing.T) {
	service, _ := newService(t)
	ctx := context.Background()

	created, _, err := service.Create(ctx, createInput())
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	summary, err := service.Summary(ctx)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.Total == 0 || summary.Active == 0 {
		t.Fatalf("summary looks empty: %+v", summary)
	}

	name := "Acme Renamed"
	expireAt := time.Now().AddDate(1, 0, 0)
	updated, err := service.Update(ctx, created.ID, tenants.UpdateInput{Name: &name, ExpireAt: &expireAt})
	if err != nil {
		t.Fatalf("update tenant: %v", err)
	}
	if updated.Name != name {
		t.Fatalf("name: want %q, got %q", name, updated.Name)
	}
	if updated.ExpireAt == nil {
		t.Fatal("expire_at should be set after renewal")
	}

	users, err := service.Users(ctx, created.ID)
	if err != nil {
		t.Fatalf("list tenant users: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("want 1 tenant user, got %d", len(users))
	}
	if len(users[0].Roles) != 1 || users[0].Roles[0] != "admin" {
		t.Fatalf("admin role missing: %+v", users[0].Roles)
	}

	_, password, err := service.ResetAdminPassword(ctx, created.ID)
	if err != nil {
		t.Fatalf("reset admin password: %v", err)
	}
	if len(password) < 16 {
		t.Fatalf("generated password looks too short: %q", password)
	}

	if _, _, err := service.RetryProvision(ctx, created.ID); err == nil {
		t.Fatal("retry on already provisioned tenant should fail")
	}
}
