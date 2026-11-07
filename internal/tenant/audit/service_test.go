package audit_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cuiwenguang/go-saas-lite/internal/tenant/audit"
	"github.com/cuiwenguang/go-saas-lite/internal/tenant/members"
	"github.com/cuiwenguang/go-saas-lite/internal/testutil"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

func TestRecordAndListByTenant(t *testing.T) {
	h := testutil.New(t)
	tenantID := h.SeedTenant(t, "audit-main")
	otherTenantID := h.SeedTenant(t, "audit-other")
	_ = h.SeedRole(t, tenantID, "member", nil)

	auditService := audit.NewService(h.Tenant)
	userID := testutil.NewUserID()
	ctx := tenancy.WithScope(context.Background(), tenancy.Scope{
		TenantID:   tenantID,
		UserID:     userID,
		ActorEmail: "admin@audit.test",
		Actor:      tenancy.ActorTenant,
	})

	memberService := members.NewService(h.Tenant, h.Identity(), nil, auditService)
	unique := strings.ReplaceAll(uuid.Must(uuid.NewV7()).String(), "-", "")
	created, err := memberService.Create(ctx, members.CreateInput{
		Email: "member-" + unique + "@audit.test",
		Role:  "member",
	})
	if err != nil {
		t.Fatalf("create member: %v", err)
	}

	items, total, err := auditService.List(ctx, audit.Filter{})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("want 1 audit entry, got total=%d len=%d", total, len(items))
	}

	entry := items[0]
	if entry.Action != audit.ActionUserCreate {
		t.Fatalf("want action %s, got %s", audit.ActionUserCreate, entry.Action)
	}
	if entry.ObjectID != created.User.ID {
		t.Fatalf("want object %s, got %s", created.User.ID, entry.ObjectID)
	}
	if entry.ActorID != userID || entry.ActorEmail != "admin@audit.test" {
		t.Fatalf("unexpected actor: %s / %s", entry.ActorID, entry.ActorEmail)
	}
	if len(entry.After) == 0 {
		t.Fatal("want after snapshot")
	}
	var after map[string]any
	if err := json.Unmarshal(entry.After, &after); err != nil {
		t.Fatalf("decode after: %v", err)
	}
	if after["role"] != "member" {
		t.Fatalf("want role member, got %v", after["role"])
	}

	// 其他租户读不到这条记录。
	otherCtx := tenancy.WithScope(context.Background(), tenancy.Scope{
		TenantID: otherTenantID,
		Actor:    tenancy.ActorTenant,
	})
	_, otherTotal, err := auditService.List(otherCtx, audit.Filter{})
	if err != nil {
		t.Fatalf("list other tenant audit: %v", err)
	}
	if otherTotal != 0 {
		t.Fatalf("tenant isolation broken, got %d entries", otherTotal)
	}

	// 分类过滤生效。
	_, filtered, err := auditService.List(ctx, audit.Filter{Category: audit.CategorySecurity})
	if err != nil {
		t.Fatalf("list filtered audit: %v", err)
	}
	if filtered != 1 {
		t.Fatalf("want 1 security entry, got %d", filtered)
	}
	_, none, err := auditService.List(ctx, audit.Filter{Category: audit.CategoryFile})
	if err != nil {
		t.Fatalf("list file audit: %v", err)
	}
	if none != 0 {
		t.Fatalf("want 0 file entries, got %d", none)
	}
}
