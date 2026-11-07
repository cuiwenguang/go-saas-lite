package mcp_test

import (
	"context"
	"testing"

	"github.com/cuiwenguang/go-saas-lite/internal/mcp"
)

func TestMemorySessionsCount(t *testing.T) {
	store := mcp.NewMemorySessions()
	ctx := context.Background()

	if err := store.Create(ctx, mcp.Session{ID: "s1", TenantID: "tenant-a"}); err != nil {
		t.Fatalf("create s1: %v", err)
	}
	if err := store.Create(ctx, mcp.Session{ID: "s2", TenantID: "tenant-a"}); err != nil {
		t.Fatalf("create s2: %v", err)
	}
	if err := store.Create(ctx, mcp.Session{ID: "s3", TenantID: "tenant-b"}); err != nil {
		t.Fatalf("create s3: %v", err)
	}

	count, err := store.Count(ctx, "tenant-a")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Fatalf("want 2 sessions for tenant-a, got %d", count)
	}

	if err := store.Delete(ctx, "tenant-a", "s1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	count, err = store.Count(ctx, "tenant-a")
	if err != nil {
		t.Fatalf("count after delete: %v", err)
	}
	if count != 1 {
		t.Fatalf("want 1 session after delete, got %d", count)
	}
}

func TestMemorySessionsRevokeClient(t *testing.T) {
	store := mcp.NewMemorySessions()
	ctx := context.Background()

	_ = store.Create(ctx, mcp.Session{ID: "s1", TenantID: "t1", UserID: "u1", ClientID: "c1"})
	_ = store.Create(ctx, mcp.Session{ID: "s2", TenantID: "t1", UserID: "u1", ClientID: "c1"})
	_ = store.Create(ctx, mcp.Session{ID: "s3", TenantID: "t1", UserID: "u1", ClientID: "c2"})

	if err := store.RevokeClient(ctx, "t1", "u1", "c1"); err != nil {
		t.Fatalf("revoke client: %v", err)
	}

	count, err := store.Count(ctx, "t1")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("want 1 remaining session, got %d", count)
	}
	if _, err := store.Get(ctx, "t1", "s3"); err != nil {
		t.Fatalf("other client session should remain: %v", err)
	}
}
