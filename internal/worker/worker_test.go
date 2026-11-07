package worker_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cuiwenguang/go-saas-lite/internal/platform/audit"
	"github.com/cuiwenguang/go-saas-lite/internal/platform/tenants"
	"github.com/cuiwenguang/go-saas-lite/internal/testutil"
	"github.com/cuiwenguang/go-saas-lite/internal/worker"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

type tenantRecord struct {
	ID        string     `gorm:"column:id;primaryKey"`
	DeletedAt *time.Time `gorm:"column:deleted_at"`
}

func (tenantRecord) TableName() string { return "tenants" }

func newWorker(h *testutil.Harness, coolingDays int) *worker.Worker {
	return worker.NewRunner(
		worker.Options{Interval: time.Minute, CoolingDays: coolingDays},
		h.Platform,
		h.Tenant,
		nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
}

// 归档且超过冷却期的租户，数据要被清掉。
func TestPurgeArchivedTenant(t *testing.T) {
	h := testutil.New(t)
	service := tenants.NewService(h.Platform, h.Tenant, audit.NewService(h.Platform), h.Identity())
	ctx := context.Background()

	hex := strings.ReplaceAll(uuid.Must(uuid.NewV7()).String(), "-", "")
	code := "worker-" + hex[len(hex)-10:]
	created, _, err := service.Create(ctx, tenants.CreateInput{
		Code:          code,
		Name:          "Worker Tenant",
		Timezone:      "Asia/Shanghai",
		Locale:        "zh-CN",
		AdminEmail:    code + "@worker.test",
		AdminPassword: "password123",
	})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	// 造一条配置，验证清理范围。
	if err := h.Tenant.Tx(testutil.Context(created.ID), func(tx tenancy.Tx) error {
		return tx.Raw("SELECT 1").Scan(new(int64))
	}); err != nil {
		t.Fatalf("probe tenant tx: %v", err)
	}

	if err := service.Archive(ctx, created.ID); err != nil {
		t.Fatalf("archive: %v", err)
	}

	// 把 updated_at 拨回冷却期之前。
	if err := h.Platform.Tx(ctx, func(tx tenancy.Tx) error {
		return tx.Where("id = ?", created.ID).Update(&tenantRecord{}, map[string]any{
			"updated_at": time.Now().AddDate(0, 0, -10),
		})
	}); err != nil {
		t.Fatalf("backdate tenant: %v", err)
	}

	purged, err := newWorker(h, 7).PurgeArchived(ctx)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if purged < 1 {
		t.Fatalf("expected at least one purged tenant, got %d", purged)
	}

	scopeCtx := testutil.Context(created.ID)

	var users, roles, bindings int64
	if err := h.Tenant.Read(scopeCtx, func(tx tenancy.Tx) error {
		if err := tx.Model(&testutil.User{}).Count(&users); err != nil {
			return err
		}
		if err := tx.Model(&testutil.Role{}).Count(&roles); err != nil {
			return err
		}
		return tx.Model(&testutil.UserRole{}).Count(&bindings)
	}); err != nil {
		t.Fatalf("count after purge: %v", err)
	}

	if users != 0 || roles != 0 || bindings != 0 {
		t.Fatalf("tenant data not purged: users=%d roles=%d bindings=%d", users, roles, bindings)
	}

	var row tenantRecord
	if err := h.PlatformGorm.WithContext(ctx).Raw("SELECT id, deleted_at FROM tenants WHERE id = ?", created.ID).Scan(&row).Error; err != nil {
		t.Fatalf("load tenant: %v", err)
	}
	if row.DeletedAt == nil {
		t.Fatal("tenant should be marked deleted_at after purge")
	}
}

// 过了 expire_at 的 active 租户要流转为 expired，未到期的不动。
func TestExpireTenants(t *testing.T) {
	h := testutil.New(t)
	ctx := context.Background()
	service := tenants.NewService(h.Platform, h.Tenant, audit.NewService(h.Platform), h.Identity())

	create := func(prefix string) string {
		hex := strings.ReplaceAll(uuid.Must(uuid.NewV7()).String(), "-", "")
		code := prefix + "-" + hex[len(hex)-10:]
		created, _, err := service.Create(ctx, tenants.CreateInput{
			Code:          code,
			Name:          "Expire Tenant",
			Timezone:      "Asia/Shanghai",
			Locale:        "zh-CN",
			AdminEmail:    code + "@" + prefix + ".test",
			AdminPassword: "password123",
		})
		if err != nil {
			t.Fatalf("create tenant: %v", err)
		}
		return created.ID
	}

	expiredID := create("expired")
	freshID := create("fresh")

	if err := h.PlatformGorm.Exec(`UPDATE tenants SET expire_at = ? WHERE id = ?`, time.Now().AddDate(0, 0, -1), expiredID).Error; err != nil {
		t.Fatalf("backdate expire_at: %v", err)
	}
	if err := h.PlatformGorm.Exec(`UPDATE tenants SET expire_at = ? WHERE id = ?`, time.Now().AddDate(0, 0, 30), freshID).Error; err != nil {
		t.Fatalf("set future expire_at: %v", err)
	}

	runner := worker.NewRunner(
		worker.Options{Interval: time.Minute, CoolingDays: 7},
		h.Platform, h.Tenant, nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	changed, err := runner.ExpireTenants(ctx)
	if err != nil {
		t.Fatalf("expire tenants: %v", err)
	}
	if changed < 1 {
		t.Fatalf("expected at least one tenant expired, got %d", changed)
	}

	statusOf := func(id string) string {
		var status string
		if err := h.PlatformGorm.Raw(`SELECT status FROM tenants WHERE id = ?`, id).Scan(&status).Error; err != nil {
			t.Fatalf("load status: %v", err)
		}
		return status
	}
	if got := statusOf(expiredID); got != "expired" {
		t.Fatalf("expired tenant status: want expired, got %q", got)
	}
	if got := statusOf(freshID); got != "active" {
		t.Fatalf("fresh tenant status: want active, got %q", got)
	}
}

// 超过保留期的审计日志要被清掉，期内的保留。
func TestPurgeAuditLogs(t *testing.T) {
	h := testutil.New(t)
	ctx := context.Background()

	oldID := uuid.Must(uuid.NewV7()).String()
	newID := uuid.Must(uuid.NewV7()).String()
	seed := func(id string, createdAt time.Time) {
		t.Helper()
		if err := h.PlatformGorm.Exec(
			`INSERT INTO system_operation_log (id, category, action, created_at) VALUES (?, 'auth', 'platform.login', ?)`,
			id, createdAt,
		).Error; err != nil {
			t.Fatalf("seed audit log: %v", err)
		}
	}
	seed(oldID, time.Now().AddDate(0, 0, -40))
	seed(newID, time.Now())

	runner := worker.NewRunner(
		worker.Options{Interval: time.Minute, CoolingDays: 7, AuditRetentionDays: 30},
		h.Platform, h.Tenant, nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	deleted, err := runner.PurgeAudit(ctx)
	if err != nil {
		t.Fatalf("purge audit: %v", err)
	}
	if deleted < 1 {
		t.Fatalf("expected at least one audit log purged, got %d", deleted)
	}

	var remaining int64
	if err := h.PlatformGorm.Raw(`SELECT COUNT(*) FROM system_operation_log WHERE id = ?`, oldID).Scan(&remaining).Error; err != nil {
		t.Fatalf("count old: %v", err)
	}
	if remaining != 0 {
		t.Fatal("old audit log should be purged")
	}
	if err := h.PlatformGorm.Raw(`SELECT COUNT(*) FROM system_operation_log WHERE id = ?`, newID).Scan(&remaining).Error; err != nil {
		t.Fatalf("count new: %v", err)
	}
	if remaining != 1 {
		t.Fatal("fresh audit log must stay")
	}
}

// 未到冷却期的归档租户不能被清理。
func TestPurgeSkipsTenantsInCoolingPeriod(t *testing.T) {
	h := testutil.New(t)
	service := tenants.NewService(h.Platform, h.Tenant, audit.NewService(h.Platform), h.Identity())
	ctx := context.Background()

	hex := strings.ReplaceAll(uuid.Must(uuid.NewV7()).String(), "-", "")
	code := "cooling-" + hex[len(hex)-10:]
	created, _, err := service.Create(ctx, tenants.CreateInput{
		Code:       code,
		Name:       "Cooling Tenant",
		Timezone:   "Asia/Shanghai",
		Locale:     "zh-CN",
		AdminEmail: code + "@cooling.test",
	})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if err := service.Archive(ctx, created.ID); err != nil {
		t.Fatalf("archive: %v", err)
	}

	if _, err := newWorker(h, 7).PurgeArchived(ctx); err != nil {
		t.Fatalf("purge: %v", err)
	}

	var users int64
	if err := h.Tenant.Read(testutil.Context(created.ID), func(tx tenancy.Tx) error {
		return tx.Model(&testutil.User{}).Count(&users)
	}); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if users != 1 {
		t.Fatalf("data should stay during cooling period, users=%d", users)
	}
}
