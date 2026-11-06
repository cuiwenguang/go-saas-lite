package testutil

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/cuiwenguang/go-saas-lite/internal/security/identity"
	"github.com/cuiwenguang/go-saas-lite/migrations"
	"github.com/cuiwenguang/go-saas-lite/pkg/migrate"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// Harness 提供两个连接池（租户侧带 Callback、平台侧不带）+ 已跑完最小迁移的库。
type Harness struct {
	Tenant       tenancy.DB
	Platform     tenancy.PlatformDB
	TenantGorm   *gorm.DB
	PlatformGorm *gorm.DB
}

// Identity 构造全局登录索引服务，供手工装配依赖的用例使用。
func (h *Harness) Identity() *identity.Service {
	return identity.NewService(h.Platform, h.Tenant)
}

// New 准备测试库：连库 → 跑平台 / 租户两套迁移 → 返回门面。
func New(t *testing.T) *Harness {
	t.Helper()

	dsn := DSN(t)

	platformGorm, err := tenancy.OpenPostgres(dsn, tenancy.Silent())
	if err != nil {
		t.Fatalf("open platform connection: %v", err)
	}

	tenantGorm, err := tenancy.OpenPostgres(dsn, tenancy.WithTenantCallbacks(), tenancy.Silent())
	if err != nil {
		t.Fatalf("open tenant connection: %v", err)
	}

	h := &Harness{
		Tenant:       tenancy.NewDB(tenantGorm),
		Platform:     tenancy.NewPlatformDB(platformGorm),
		TenantGorm:   tenantGorm,
		PlatformGorm: platformGorm,
	}
	h.migrate(t)

	return h
}

func (h *Harness) migrate(t *testing.T) {
	t.Helper()

	runner := migrate.New(h.PlatformGorm, migrations.FS)
	for _, group := range migrate.Groups() {
		if err := runner.Up(context.Background(), group); err != nil {
			t.Fatalf("apply %s migrations: %v", group, err)
		}
	}
}

// SeedTenant 通过平台门面插入租户，返回租户 ID。
// code 只作可读标签，实际值追加随机后缀，保证多次运行 / 多个用例共用同一库时不冲突。
func (h *Harness) SeedTenant(t *testing.T, code string) string {
	t.Helper()

	id := uuid.Must(uuid.NewV7()).String()
	hex := strings.ReplaceAll(id, "-", "")
	uniqueCode := code + "-" + hex[len(hex)-12:]

	err := h.Platform.Tx(context.Background(), func(tx tenancy.Tx) error {
		return tx.Create(&Tenant{ID: id, Code: uniqueCode, Name: code, Status: "active"})
	})
	if err != nil {
		t.Fatalf("seed tenant %s: %v", code, err)
	}

	return id
}

// Context 构造带租户 Scope 的 ctx。
func Context(tenantID string) context.Context {
	return tenancy.WithScope(context.Background(), tenancy.Scope{
		TenantID: tenantID,
		Actor:    tenancy.ActorTenant,
	})
}

// CreateWidget 在当前租户下插入一条记录，返回 ID。
func (h *Harness) CreateWidget(t *testing.T, tenantID, name string) string {
	t.Helper()

	id := uuid.Must(uuid.NewV7()).String()
	err := h.Tenant.Tx(Context(tenantID), func(tx tenancy.Tx) error {
		return tx.Create(&Widget{ID: id, Name: name})
	})
	if err != nil {
		t.Fatalf("create widget in %s: %v", tenantID, err)
	}

	return id
}

// CountWidgets 统计当前租户可见的记录数。
func (h *Harness) CountWidgets(t *testing.T, tenantID string) int64 {
	t.Helper()

	var count int64
	err := h.Tenant.Read(Context(tenantID), func(tx tenancy.Tx) error {
		return tx.Model(&Widget{}).Count(&count)
	})
	if err != nil {
		t.Fatalf("count widgets in %s: %v", tenantID, err)
	}

	return count
}

// MustWidget 断言当前租户能看到指定记录。
func (h *Harness) MustWidget(t *testing.T, tenantID, widgetID string) *Widget {
	t.Helper()

	var widget Widget
	err := h.Tenant.Read(Context(tenantID), func(tx tenancy.Tx) error {
		return tx.Where("id = ?", widgetID).First(&widget)
	})
	if err != nil {
		t.Fatalf("load widget %s in %s: %v", widgetID, tenantID, err)
	}

	return &widget
}

// MustNotSeeWidget 断言当前租户看不到指定记录。
func (h *Harness) MustNotSeeWidget(t *testing.T, tenantID, widgetID string) {
	t.Helper()

	var widget Widget
	err := h.Tenant.Read(Context(tenantID), func(tx tenancy.Tx) error {
		return tx.Where("id = ?", widgetID).First(&widget)
	})
	if err == nil {
		t.Fatalf("tenant %s unexpectedly sees widget %s owned by another tenant", tenantID, widgetID)
	}
	if err != gorm.ErrRecordNotFound {
		t.Fatalf("want ErrRecordNotFound, got %v", err)
	}
}

// RawCountAll 用原生 SQL 统计一条租户可见记录，验证 3.2.1 的写法。
func (h *Harness) RawCountAll(t *testing.T, tenantID string) int64 {
	t.Helper()

	var total int64
	err := h.Tenant.Read(Context(tenantID), func(tx tenancy.Tx) error {
		return tx.Raw("SELECT COUNT(*) FROM widgets WHERE tenant_id = ?", tenantID).Scan(&total)
	})
	if err != nil {
		t.Fatalf("raw count in %s: %v", tenantID, err)
	}

	return total
}

// Describe 便于失败信息定位。
func Describe(tenantID string) string {
	return fmt.Sprintf("tenant(%s)", tenantID)
}
