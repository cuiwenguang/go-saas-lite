package migrations_test

import (
	"testing"

	"github.com/cuiwenguang/go-saas-lite/migrations"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// readme 必查清单 14：双向一致性，防止业务表被建到平台组（永不隔离）。
func TestPlatformMigrationsAreWhitelisted(t *testing.T) {
	t.Parallel()

	names, err := migrations.TableNames(migrations.PlatformDir)
	if err != nil {
		t.Fatalf("parse platform migrations: %v", err)
	}
	if len(names) == 0 {
		t.Fatal("platform migrations contain no CREATE TABLE")
	}

	for _, name := range names {
		if !tenancy.IsPlatformTable(name) {
			t.Errorf("platform migration creates %q, which is not in PlatformTables", name)
		}
	}
}

func TestTenantMigrationsAreNotPlatformTables(t *testing.T) {
	t.Parallel()

	names, err := migrations.TableNames(migrations.TenantDir)
	if err != nil {
		t.Fatalf("parse tenant migrations: %v", err)
	}
	if len(names) == 0 {
		t.Fatal("tenant migrations contain no CREATE TABLE")
	}

	for _, name := range names {
		if tenancy.IsPlatformTable(name) {
			t.Errorf("tenant migration creates platform table %q; it would lose tenant isolation", name)
		}
	}
}
