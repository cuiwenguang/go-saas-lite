package app_test

import (
	"context"
	"testing"
	"time"

	"go.uber.org/fx"

	"github.com/cuiwenguang/go-saas-lite/internal/app"
)

// 验证 fx 生命周期：OnStart 能起服务，OnStop 能在超时内优雅关闭。
func TestAppStartsAndStops(t *testing.T) {
	cfg := testConfig(t)
	cfg.Server.Addr = "127.0.0.1:0"
	cfg.Trace.Exporter = "none"

	instance := fx.New(app.New(cfg), fx.NopLogger)

	startCtx, cancelStart := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelStart()

	if err := instance.Start(startCtx); err != nil {
		t.Fatalf("start app: %v", err)
	}

	stopCtx, cancelStop := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelStop()

	if err := instance.Stop(stopCtx); err != nil {
		t.Fatalf("stop app: %v", err)
	}
}
