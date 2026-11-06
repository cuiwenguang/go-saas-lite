package main

import (
	"flag"
	"fmt"
	"os"

	"go.uber.org/fx"

	"github.com/cuiwenguang/go-saas-lite/internal/app"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	flag.Parse()

	cfg, err := app.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	fx.New(app.New(cfg)).Run()
}
