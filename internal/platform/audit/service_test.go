package audit_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cuiwenguang/go-saas-lite/internal/platform/audit"
	"github.com/cuiwenguang/go-saas-lite/internal/testutil"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// 写入后能按分类 / 级别 / 操作人筛出来，且 before / after 原样返回。
func TestRecordAndList(t *testing.T) {
	h := testutil.New(t)
	service := audit.NewService(h.Platform)

	adminID := uuid.Must(uuid.NewV7()).String()
	hex := strings.ReplaceAll(adminID, "-", "")
	actorEmail := "admin-" + hex[:8] + "@audit.test"
	ctx := tenancy.WithScope(context.Background(), tenancy.Scope{
		Actor:      tenancy.ActorPlatform,
		UserID:     adminID,
		ActorEmail: actorEmail,
	})

	tenantID := uuid.Must(uuid.NewV7()).String()
	if err := service.Record(ctx, audit.Entry{
		Category:   audit.CategoryTenant,
		Action:     audit.ActionTenantCreate,
		ObjectType: "tenant",
		ObjectID:   tenantID,
		Summary:    "开通租户 Acme",
		After:      map[string]any{"name": "Acme"},
	}); err != nil {
		t.Fatalf("record create: %v", err)
	}

	if err := service.Record(ctx, audit.Entry{
		Category:   audit.CategoryTenant,
		Action:     audit.ActionTenantResetPass,
		Level:      audit.LevelWarn,
		ObjectType: "tenant",
		ObjectID:   tenantID,
		Summary:    "重置租户管理员密码 Acme",
	}); err != nil {
		t.Fatalf("record reset: %v", err)
	}

	if err := service.Record(ctx, audit.Entry{
		Category:   audit.CategoryAuth,
		Action:     audit.ActionLoginFailed,
		Level:      audit.LevelWarn,
		Result:     audit.ResultFailed,
		ActorType:  audit.ActorAdmin,
		ActorEmail: actorEmail,
		Summary:    "平台登录失败",
	}); err != nil {
		t.Fatalf("record login failed: %v", err)
	}

	items, total, err := service.List(ctx, audit.Filter{Category: audit.CategoryTenant, ObjectID: tenantID})
	if err != nil {
		t.Fatalf("list by category: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("want 2 tenant logs, got total=%d items=%d", total, len(items))
	}
	for _, item := range items {
		if item.ActorEmail != actorEmail {
			t.Fatalf("actor email should come from scope, got %q", item.ActorEmail)
		}
	}
	if len(items[0].After) == 0 && len(items[1].After) == 0 {
		t.Fatal("before/after payload should be persisted")
	}

	warns, warnTotal, err := service.List(ctx, audit.Filter{Level: audit.LevelWarn, ObjectID: tenantID})
	if err != nil {
		t.Fatalf("list by level: %v", err)
	}
	if warnTotal != 1 || len(warns) != 1 {
		t.Fatalf("want 1 warn log for this tenant, got total=%d", warnTotal)
	}

	byActor, _, err := service.List(ctx, audit.Filter{Actor: actorEmail})
	if err != nil {
		t.Fatalf("list by actor: %v", err)
	}
	if len(byActor) != 3 {
		t.Fatalf("want 3 logs by actor, got %d", len(byActor))
	}

	byObject, _, err := service.List(ctx, audit.Filter{ObjectID: tenantID})
	if err != nil {
		t.Fatalf("list by object: %v", err)
	}
	if len(byObject) != 2 {
		t.Fatalf("want 2 logs for tenant object, got %d", len(byObject))
	}
}
