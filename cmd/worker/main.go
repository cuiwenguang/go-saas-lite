package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"go.uber.org/fx"

	"github.com/cuiwenguang/go-saas-lite/internal/app"
	"github.com/cuiwenguang/go-saas-lite/internal/worker"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	flag.Parse()

	cfg, err := app.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	opts := worker.Options{
		Interval:           time.Duration(cfg.Worker.IntervalSeconds) * time.Second,
		CoolingDays:        cfg.Worker.CoolingDays,
		AuditRetentionDays: cfg.Audit.RetentionDays,
	}

	fx.New(fx.Options(
		app.Core(cfg),
		fx.Supply(opts),
		fx.Provide(worker.New),
		fx.Invoke(func(*worker.Worker) {}),
	)).Run()
}
