package file_test

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/cuiwenguang/go-saas-lite/internal/tenant/file"
	"github.com/cuiwenguang/go-saas-lite/internal/testutil"
	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/storage"
)

func TestPurgeRemovesTenantFiles(t *testing.T) {
	h := testutil.New(t)

	local, err := storage.NewLocal(storage.LocalOptions{
		Root:       t.TempDir(),
		BaseURL:    "/api/files/download",
		SigningKey: "test-signing-key",
		MaxBytes:   1 << 20,
	})
	if err != nil {
		t.Fatalf("new local: %v", err)
	}
	service := file.NewService(h.Tenant, local, nil, nil, nil, file.Options{MaxUploadBytes: 1 << 20})

	tenantA := h.SeedTenant(t, "purge-a")
	tenantB := h.SeedTenant(t, "purge-b")
	ctxA := testutil.Context(tenantA)
	ctxB := testutil.Context(tenantB)

	fileA, err := service.Upload(ctxA, "docs", "a.txt", "text/plain", 5, strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("upload A: %v", err)
	}
	fileB, err := service.Upload(ctxB, "docs", "b.txt", "text/plain", 5, strings.NewReader("world"))
	if err != nil {
		t.Fatalf("upload B: %v", err)
	}

	if err := service.Purge(ctxA, tenantA); err != nil {
		t.Fatalf("purge: %v", err)
	}

	itemsA, err := service.List(ctxA, "")
	if err != nil {
		t.Fatalf("list A: %v", err)
	}
	if len(itemsA) != 0 {
		t.Fatalf("tenant A metadata should be purged, got %+v", itemsA)
	}
	if ok, err := local.Exists(ctxA, fileA.Key); err != nil || ok {
		t.Fatalf("tenant A object should be deleted, exists=%v err=%v", ok, err)
	}

	itemsB, err := service.List(ctxB, "")
	if err != nil {
		t.Fatalf("list B: %v", err)
	}
	if len(itemsB) != 1 || itemsB[0].ID != fileB.ID {
		t.Fatalf("tenant B files must survive, got %+v", itemsB)
	}
}

func TestUploadListIsolationAndDelete(t *testing.T) {
	h := testutil.New(t)

	local, err := storage.NewLocal(storage.LocalOptions{
		Root:       t.TempDir(),
		BaseURL:    "/api/files/download",
		SigningKey: "test-signing-key",
		MaxBytes:   1 << 20,
	})
	if err != nil {
		t.Fatalf("new local: %v", err)
	}
	service := file.NewService(h.Tenant, local, nil, nil, nil, file.Options{MaxUploadBytes: 1 << 20})

	tenantA := h.SeedTenant(t, "files-a")
	tenantB := h.SeedTenant(t, "files-b")
	ctxA := testutil.Context(tenantA)
	ctxB := testutil.Context(tenantB)

	out, err := service.Upload(ctxA, "docs", "a.txt", "text/plain", 5, strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if out.Key == "" || out.SizeBytes != 5 {
		t.Fatalf("unexpected output: %+v", out)
	}

	itemsA, err := service.List(ctxA, "docs")
	if err != nil {
		t.Fatalf("list A: %v", err)
	}
	if len(itemsA) != 1 || itemsA[0].ID != out.ID {
		t.Fatalf("tenant A should see its file, got %+v", itemsA)
	}

	itemsB, err := service.List(ctxB, "docs")
	if err != nil {
		t.Fatalf("list B: %v", err)
	}
	if len(itemsB) != 0 {
		t.Fatalf("tenant B must not see tenant A files, got %+v", itemsB)
	}

	if err := service.Delete(ctxB, out.ID); !apperr.Is(err, apperr.CodeNotFound) {
		t.Fatalf("cross-tenant delete should be not found, got %v", err)
	}

	signed, err := service.SignedURL(ctxA, out.ID, time.Minute)
	if err != nil {
		t.Fatalf("signed url: %v", err)
	}
	if !strings.Contains(signed, "sig=") {
		t.Fatalf("unexpected signed url: %s", signed)
	}

	reader, err := service.Open(context.Background(), out.Key)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	body, _ := io.ReadAll(reader)
	_ = reader.Close()
	if string(body) != "hello" {
		t.Fatalf("unexpected body %q", body)
	}

	if err := service.Delete(ctxA, out.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	itemsA, err = service.List(ctxA, "docs")
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(itemsA) != 0 {
		t.Fatalf("file should be deleted, got %+v", itemsA)
	}
}
