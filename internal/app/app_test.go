package app_test

import (
	"os"
	"testing"

	"go.uber.org/fx"

	"github.com/cuiwenguang/go-saas-lite/internal/app"
)

// 依赖图错误是运行时 panic，不是编译错误，所以 CI 必须跑这条校验（readme 3.3）。
func TestAppGraphIsValid(t *testing.T) {
	t.Parallel()

	if err := fx.ValidateApp(app.New(testConfig(t)), fx.NopLogger); err != nil {
		t.Fatalf("invalid fx dependency graph: %v", err)
	}
}

// testConfig 用测试库 DSN 覆盖默认配置；没有 DSN 时跳过。
func testConfig(t *testing.T) app.Config {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is required for app wiring tests")
	}

	cfg := app.DefaultConfig()
	cfg.Database.PlatformDSN = dsn
	cfg.Database.TenantDSN = dsn
	cfg.Auth.Secret = "test-secret"
	cfg.Storage.Local.Root = t.TempDir()

	return cfg
}
