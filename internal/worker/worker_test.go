package worker_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cuiwenguang/go-saas-lite/internal/platform/tenant"
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
	service := tenant.NewService(h.Platform, h.Tenant)
	ctx := context.Background()

	hex := strings.ReplaceAll(uuid.Must(uuid.NewV7()).String(), "-", "")
	created, _, err := service.Create(ctx, tenant.CreateInput{
		Code:          "worker-" + hex[len(hex)-10:],
		Name:          "Worker Tenant",
		Timezone:      "Asia/Shanghai",
		Locale:        "zh-CN",
		AdminEmail:    "admin@worker.test",
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

// 未到冷却期的归档租户不能被清理。
func TestPurgeSkipsTenantsInCoolingPeriod(t *testing.T) {
	h := testutil.New(t)
	service := tenant.NewService(h.Platform, h.Tenant)
	ctx := context.Background()

	hex := strings.ReplaceAll(uuid.Must(uuid.NewV7()).String(), "-", "")
	created, _, err := service.Create(ctx, tenant.CreateInput{
		Code:       "cooling-" + hex[len(hex)-10:],
		Name:       "Cooling Tenant",
		Timezone:   "Asia/Shanghai",
		Locale:     "zh-CN",
		AdminEmail: "admin@cooling.test",
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
