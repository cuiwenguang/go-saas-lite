package conf_test

import (
	"context"
	"testing"

	"github.com/cuiwenguang/go-saas-lite/internal/tenant/conf"
	"github.com/cuiwenguang/go-saas-lite/internal/testutil"
	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
)

func TestSetGetListDelete(t *testing.T) {
	h := testutil.New(t)
	store := conf.NewStore(h.Tenant)
	ctx := testutil.Context(h.SeedTenant(t, "conf-a"))

	if err := store.Set(ctx, "general", "site_name", "Acme"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := store.Set(ctx, "general", "site_name", "Acme 2"); err != nil {
		t.Fatalf("overwrite: %v", err)
	}

	value, err := store.Get(ctx, "general", "site_name")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if value != "Acme 2" {
		t.Fatalf("want Acme 2, got %q", value)
	}

	entries, err := store.List(ctx, "general")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}

	if err := store.Delete(ctx, "general", "site_name"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.Get(ctx, "general", "site_name"); !apperr.Is(err, apperr.CodeNotFound) {
		t.Fatalf("want CodeNotFound after delete, got %v", err)
	}
}

// 配置必须按租户隔离：A 写的 key，B 读不到。
func TestConfigIsTenantScoped(t *testing.T) {
	h := testutil.New(t)
	store := conf.NewStore(h.Tenant)

	tenantA := h.SeedTenant(t, "conf-a")
	tenantB := h.SeedTenant(t, "conf-b")

	if err := store.Set(testutil.Context(tenantA), "general", "feature_flag", "on"); err != nil {
		t.Fatalf("set in A: %v", err)
	}

	ctxB := testutil.Context(tenantB)
	if _, err := store.Get(ctxB, "general", "feature_flag"); !apperr.Is(err, apperr.CodeNotFound) {
		t.Fatalf("tenant B must not read tenant A config, got %v", err)
	}

	entries, err := store.List(ctxB, "general")
	if err != nil {
		t.Fatalf("list in B: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("tenant B should see 0 entries, got %d", len(entries))
	}

	// 同名 key 在两个租户下互不覆盖。
	if err := store.Set(ctxB, "general", "feature_flag", "off"); err != nil {
		t.Fatalf("set in B: %v", err)
	}
	valueA, err := store.Get(testutil.Context(tenantA), "general", "feature_flag")
	if err != nil || valueA != "on" {
		t.Fatalf("tenant A value changed: %q, %v", valueA, err)
	}
}

func TestContextRequired(t *testing.T) {
	h := testutil.New(t)
	store := conf.NewStore(h.Tenant)

	if err := store.Set(context.Background(), "general", "k", "v"); err == nil {
		t.Fatal("missing scope must be rejected")
	}
}
