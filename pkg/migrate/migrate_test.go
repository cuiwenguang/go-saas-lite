package migrate_test

import (
	"context"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/cuiwenguang/go-saas-lite/internal/testutil"
	"github.com/cuiwenguang/go-saas-lite/migrations"
	"github.com/cuiwenguang/go-saas-lite/pkg/migrate"
)

func copyFS(t *testing.T) fstest.MapFS {
	t.Helper()

	files := fstest.MapFS{}
	for _, group := range migrate.Groups() {
		entries, err := fs.ReadDir(migrations.FS, string(group))
		if err != nil {
			t.Fatalf("read %s: %v", group, err)
		}
		for _, entry := range entries {
			content, readErr := fs.ReadFile(migrations.FS, string(group)+"/"+entry.Name())
			if readErr != nil {
				t.Fatalf("read %s/%s: %v", group, entry.Name(), readErr)
			}
			files[string(group)+"/"+entry.Name()] = &fstest.MapFile{Data: content}
		}
	}

	return files
}

func TestUpIsIdempotent(t *testing.T) {
	h := testutil.New(t)
	runner := migrate.New(h.PlatformGorm, migrations.FS)

	for round := 0; round < 2; round++ {
		for _, group := range migrate.Groups() {
			if err := runner.Up(context.Background(), group); err != nil {
				t.Fatalf("round %d apply %s: %v", round, group, err)
			}
		}
	}
}

func TestStatusReportsApplied(t *testing.T) {
	h := testutil.New(t)
	runner := migrate.New(h.PlatformGorm, migrations.FS)

	for _, group := range migrate.Groups() {
		statuses, err := runner.Status(context.Background(), group)
		if err != nil {
			t.Fatalf("status %s: %v", group, err)
		}
		if len(statuses) == 0 {
			t.Fatalf("%s has no migrations", group)
		}
		for _, status := range statuses {
			if !status.Applied || status.Drifted {
				t.Fatalf("%s v%d unexpected status: applied=%v drifted=%v",
					group, status.Version, status.Applied, status.Drifted)
			}
		}
	}
}

// 脚本改动但未重新迁移时，Verify 必须拒绝启动。
func TestVerifyDetectsChecksumDrift(t *testing.T) {
	h := testutil.New(t)

	files := copyFS(t)
	original := files["tenant/00001_init.sql"].Data
	files["tenant/00001_init.sql"] = &fstest.MapFile{Data: append(original, []byte("\n-- tampered\n")...)}

	runner := migrate.New(h.PlatformGorm, files)
	err := runner.Verify(context.Background(), migrate.Tenant)
	if err == nil {
		t.Fatal("want checksum mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}

	// 平台组没动过，仍然通过。
	if verifyErr := runner.Verify(context.Background(), migrate.Platform); verifyErr != nil {
		t.Fatalf("platform should stay valid: %v", verifyErr)
	}
}
