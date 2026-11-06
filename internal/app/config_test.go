package app_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cuiwenguang/go-saas-lite/internal/app"
)

// yaml 与 env 都要能绑定下划线键，且 env 优先。
func TestLoadConfigBindsYAMLAndEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := []byte(`
server:
  addr: ":9090"
  read_header_timeout_sec: 7
log:
  level: debug
auth:
  access_ttl_seconds: 120
audit:
  retention_days: 45
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("SAAS_SERVER__ADDR", ":9091")
	t.Setenv("SAAS_DATABASE__PLATFORM_DSN", "postgres://platform")
	t.Setenv("SAAS_DATABASE__TENANT_DSN", "postgres://tenant")

	cfg, err := app.LoadConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Server.Addr != ":9091" {
		t.Fatalf("env should override yaml: want :9091, got %q", cfg.Server.Addr)
	}
	if cfg.Server.ReadHeaderTimeoutSec != 7 {
		t.Fatalf("yaml underscore key not bound: want 7, got %d", cfg.Server.ReadHeaderTimeoutSec)
	}
	if cfg.Log.Level != "debug" {
		t.Fatalf("log level: want debug, got %q", cfg.Log.Level)
	}
	if cfg.Auth.AccessTTLSeconds != 120 {
		t.Fatalf("auth ttl: want 120, got %d", cfg.Auth.AccessTTLSeconds)
	}
	if cfg.Audit.RetentionDays != 45 {
		t.Fatalf("audit retention: want 45, got %d", cfg.Audit.RetentionDays)
	}
	if cfg.Database.PlatformDSN != "postgres://platform" || cfg.Database.TenantDSN != "postgres://tenant" {
		t.Fatalf("dsn env not bound: %+v", cfg.Database)
	}
}

func TestLoadConfigRequiresDSN(t *testing.T) {
	t.Setenv("SAAS_DATABASE__PLATFORM_DSN", "")
	t.Setenv("SAAS_DATABASE__TENANT_DSN", "")

	if _, err := app.LoadConfig(""); err == nil {
		t.Fatal("missing database dsn must be rejected")
	}
}
