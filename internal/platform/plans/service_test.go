package plans_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cuiwenguang/go-saas-lite/internal/platform/audit"
	"github.com/cuiwenguang/go-saas-lite/internal/platform/plans"
	"github.com/cuiwenguang/go-saas-lite/internal/quota"
	"github.com/cuiwenguang/go-saas-lite/internal/testutil"
)

func newService(t *testing.T) *plans.Service {
	t.Helper()

	h := testutil.New(t)
	return plans.NewService(h.Platform, audit.NewService(h.Platform))
}

func TestCreateListUpdatePlan(t *testing.T) {
	service := newService(t)
	ctx := context.Background()

	hex := strings.ReplaceAll(uuid.Must(uuid.NewV7()).String(), "-", "")
	code := "test-" + hex[:8]
	limits := quota.Limits{MaxUsers: 5, StorageGB: 10, APIQPS: 20, MCPCalls: 1_000, MCPTokens: 2_000, MCPSessions: 3}

	created, err := service.Create(ctx, plans.CreateInput{Code: code, Name: "测试套餐", Quota: limits})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if created.Code != code || created.Quota.MaxUsers != 5 || !created.Enabled {
		t.Fatalf("unexpected created plan: %+v", created)
	}

	if _, err := service.Create(ctx, plans.CreateInput{Code: code, Name: "重复", Quota: limits}); err == nil {
		t.Fatal("duplicate plan code should fail")
	}

	items, err := service.List(ctx)
	if err != nil {
		t.Fatalf("list plans: %v", err)
	}
	found := false
	for _, item := range items {
		if item.ID == created.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("created plan missing from list")
	}

	name := "改名套餐"
	enabled := false
	updated, err := service.Update(ctx, created.ID, plans.UpdateInput{Name: &name, Enabled: &enabled})
	if err != nil {
		t.Fatalf("update plan: %v", err)
	}
	if updated.Name != name || updated.Enabled {
		t.Fatalf("unexpected updated plan: %+v", updated)
	}

	negative := quota.Limits{MaxUsers: -1}
	if _, err := service.Update(ctx, created.ID, plans.UpdateInput{Quota: &negative}); err == nil {
		t.Fatal("negative quota should fail")
	}
}
