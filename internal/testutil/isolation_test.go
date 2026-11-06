package testutil_test

import (
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/cuiwenguang/go-saas-lite/internal/testutil"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testutil.Stop()
	os.Exit(code)
}

// A 租户看到的数据，B 租户一律看不到、改不到、删不到。
func TestCrossTenantIsolation(t *testing.T) {
	h := testutil.New(t)

	tenantA := h.SeedTenant(t, "tenant-a")
	tenantB := h.SeedTenant(t, "tenant-b")

	widgetA := h.CreateWidget(t, tenantA, "widget-a")

	if got := h.CountWidgets(t, tenantA); got != 1 {
		t.Fatalf("tenant A count: want 1, got %d", got)
	}
	if got := h.CountWidgets(t, tenantB); got != 0 {
		t.Fatalf("tenant B count: want 0, got %d", got)
	}

	h.MustWidget(t, tenantA, widgetA)
	h.MustNotSeeWidget(t, tenantB, widgetA)

	// B 用同样的条件更新，不应命中 A 的数据。
	err := h.Tenant.Tx(testutil.Context(tenantB), func(tx tenancy.Tx) error {
		return tx.Where("id = ?", widgetA).Update(&testutil.Widget{}, map[string]any{"name": "hacked"})
	})
	if err != nil {
		t.Fatalf("tenant B update: %v", err)
	}
	if got := h.MustWidget(t, tenantA, widgetA).Name; got != "widget-a" {
		t.Fatalf("tenant B modified tenant A data: name=%q", got)
	}

	// B 用同样的主键删除，不应命中 A 的数据。
	err = h.Tenant.Tx(testutil.Context(tenantB), func(tx tenancy.Tx) error {
		return tx.Delete(&testutil.Widget{ID: widgetA})
	})
	if err != nil {
		t.Fatalf("tenant B delete: %v", err)
	}
	h.MustWidget(t, tenantA, widgetA)
}

// tenant_id 由 Scope 决定，业务传入的值会被覆盖。
func TestTenantIDComesFromScope(t *testing.T) {
	h := testutil.New(t)

	tenantA := h.SeedTenant(t, "tenant-a")
	tenantB := h.SeedTenant(t, "tenant-b")

	widget := &testutil.Widget{ID: uuid.Must(uuid.NewV7()).String(), Name: "preset", TenantID: tenantB}
	err := h.Tenant.Tx(testutil.Context(tenantA), func(tx tenancy.Tx) error {
		return tx.Create(widget)
	})
	if err != nil {
		t.Fatalf("create with preset tenant id: %v", err)
	}

	if widget.TenantID != tenantA {
		t.Fatalf("tenant_id should be overwritten by scope: want %s, got %s", tenantA, widget.TenantID)
	}
	h.MustNotSeeWidget(t, tenantB, widget.ID)
}

// 缺失 Scope 时数据库门面直接拒绝，不允许隐式跨租户。
func TestMissingScopeIsRejected(t *testing.T) {
	h := testutil.New(t)

	var count int64
	err := h.Tenant.Read(t.Context(), func(tx tenancy.Tx) error {
		return tx.Model(&testutil.Widget{}).Count(&count)
	})
	if !errors.Is(err, tenancy.ErrNoScope) {
		t.Fatalf("want ErrNoScope, got %v", err)
	}
}

// 更新 / 删除缺少条件时必须报错，避免整租户被误改。
func TestUpdateAndDeleteRequireCondition(t *testing.T) {
	h := testutil.New(t)

	tenantA := h.SeedTenant(t, "tenant-a")
	h.CreateWidget(t, tenantA, "widget-a")

	err := h.Tenant.Tx(testutil.Context(tenantA), func(tx tenancy.Tx) error {
		return tx.Update(&testutil.Widget{}, map[string]any{"name": "bulk"})
	})
	if !errors.Is(err, tenancy.ErrMissingCondition) {
		t.Fatalf("update without condition: want ErrMissingCondition, got %v", err)
	}

	err = h.Tenant.Tx(testutil.Context(tenantA), func(tx tenancy.Tx) error {
		return tx.Delete(&testutil.Widget{})
	})
	if !errors.Is(err, tenancy.ErrMissingCondition) {
		t.Fatalf("delete without condition: want ErrMissingCondition, got %v", err)
	}
}

// 原生 SQL 按规范显式写租户条件时，只能看到本租户数据。
func TestRawSQLWithExplicitTenantCondition(t *testing.T) {
	h := testutil.New(t)

	tenantA := h.SeedTenant(t, "tenant-a")
	tenantB := h.SeedTenant(t, "tenant-b")

	h.CreateWidget(t, tenantA, "widget-a-1")
	h.CreateWidget(t, tenantA, "widget-a-2")
	h.CreateWidget(t, tenantB, "widget-b-1")

	if got := h.RawCountAll(t, tenantA); got != 2 {
		t.Fatalf("raw count for tenant A: want 2, got %d", got)
	}
	if got := h.RawCountAll(t, tenantB); got != 1 {
		t.Fatalf("raw count for tenant B: want 1, got %d", got)
	}
}

// Raw 不能用于写操作。
func TestRawWriteIsRejected(t *testing.T) {
	h := testutil.New(t)

	tenantA := h.SeedTenant(t, "tenant-a")

	err := h.Tenant.Tx(testutil.Context(tenantA), func(tx tenancy.Tx) error {
		return tx.Raw("DELETE FROM widgets WHERE tenant_id = ?", tenantA).Delete(&testutil.Widget{})
	})
	if !errors.Is(err, tenancy.ErrRawWrite) {
		t.Fatalf("want ErrRawWrite, got %v", err)
	}
}

// 平台组表不允许经租户门面访问，必须直接报错。
func TestPlatformTableViaTenantDBIsRejected(t *testing.T) {
	h := testutil.New(t)

	tenantA := h.SeedTenant(t, "tenant-a")

	var tenants []testutil.Tenant
	err := h.Tenant.Read(testutil.Context(tenantA), func(tx tenancy.Tx) error {
		return tx.Find(&tenants)
	})
	if !errors.Is(err, tenancy.ErrNotTenantTable) {
		t.Fatalf("want ErrNotTenantTable, got %v", err)
	}
}
