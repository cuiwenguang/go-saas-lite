package storage_test

import (
	"context"
	"errors"
	"io"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/cuiwenguang/go-saas-lite/pkg/storage"
)

func newLocal(t *testing.T, maxBytes int64) *storage.Local {
	t.Helper()

	local, err := storage.NewLocal(storage.LocalOptions{
		Root:       t.TempDir(),
		BaseURL:    "http://example.test/api/files/download",
		SigningKey: "test-signing-key",
		MaxBytes:   maxBytes,
	})
	if err != nil {
		t.Fatalf("new local: %v", err)
	}

	return local
}

func TestLocalPutGetDeleteAndSignedURL(t *testing.T) {
	ctx := context.Background()
	local := newLocal(t, 1024)
	key := "tenant/t1/docs/a.txt"

	if err := local.Put(ctx, key, strings.NewReader("hello"), 5, "text/plain"); err != nil {
		t.Fatalf("put: %v", err)
	}

	reader, err := local.Get(ctx, key)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	body, _ := io.ReadAll(reader)
	_ = reader.Close()
	if string(body) != "hello" {
		t.Fatalf("unexpected body %q", body)
	}

	exists, err := local.Exists(ctx, key)
	if err != nil || !exists {
		t.Fatalf("exists: %v %v", exists, err)
	}

	signed, err := local.SignedURL(ctx, key, time.Minute)
	if err != nil {
		t.Fatalf("signed url: %v", err)
	}
	parsed, err := url.Parse(signed)
	if err != nil {
		t.Fatalf("parse signed url: %v", err)
	}
	query := parsed.Query()
	if err := local.Verify(query.Get("key"), query.Get("expires"), query.Get("sig")); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if err := local.Verify(key, query.Get("expires"), "bad"); !errors.Is(err, storage.ErrSignatureInvalid) {
		t.Fatalf("want ErrSignatureInvalid, got %v", err)
	}
	if err := local.Verify(key, "1", "bad"); !errors.Is(err, storage.ErrSignatureExpired) {
		t.Fatalf("want ErrSignatureExpired, got %v", err)
	}

	if err := local.Delete(ctx, key); err != nil {
		t.Fatalf("delete: %v", err)
	}
	exists, err = local.Exists(ctx, key)
	if err != nil || exists {
		t.Fatalf("object should be gone: %v %v", exists, err)
	}
}

func TestLocalRejectsInvalidKeysAndOversize(t *testing.T) {
	ctx := context.Background()
	local := newLocal(t, 4)

	for _, key := range []string{"", "../escape", "/absolute", "a\\b"} {
		if _, err := local.Get(ctx, key); !errors.Is(err, storage.ErrInvalidKey) {
			t.Fatalf("key %q should be rejected, got %v", key, err)
		}
	}

	err := local.Put(ctx, "tenant/t1/big.bin", strings.NewReader("12345"), 5, "application/octet-stream")
	if !errors.Is(err, storage.ErrTooLarge) {
		t.Fatalf("want ErrTooLarge, got %v", err)
	}
}
