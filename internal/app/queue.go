package app

import (
	"context"

	"github.com/hibiken/asynq"
	"go.uber.org/fx"

	"github.com/cuiwenguang/go-saas-lite/internal/worker"
	"github.com/cuiwenguang/go-saas-lite/pkg/queue"
)

// newQueueClient 创建 asynq 生产者；未启用或未配置 Redis 时返回 nil。
func newQueueClient(lc fx.Lifecycle, cfg Config) (*queue.Client, error) {
	if !cfg.Queue.Enabled || cfg.Redis.Addr == "" {
		return nil, nil
	}

	client := queue.NewClient(asynq.RedisClientOpt{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	lc.Append(fx.Hook{
		OnStop: func(context.Context) error {
			return client.Close()
		},
	})

	return client, nil
}

// queueOptions 把进程配置转换为 worker 的队列参数。
func queueOptions(cfg Config) worker.QueueOptions {
	return worker.QueueOptions{
		Enabled:       cfg.Queue.Enabled,
		Concurrency:   cfg.Queue.Concurrency,
		Heartbeat:     cfg.Queue.Heartbeat,
		RedisAddr:     cfg.Redis.Addr,
		RedisPassword: cfg.Redis.Password,
		RedisDB:       cfg.Redis.DB,
	}
}
