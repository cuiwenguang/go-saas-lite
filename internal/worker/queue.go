package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/hibiken/asynq"
	"go.uber.org/fx"

	"github.com/cuiwenguang/go-saas-lite/internal/platform/tenants"
	"github.com/cuiwenguang/go-saas-lite/internal/tasktype"
	"github.com/cuiwenguang/go-saas-lite/pkg/queue"
	"github.com/cuiwenguang/go-saas-lite/pkg/sms"
	"github.com/cuiwenguang/go-saas-lite/pkg/storage"
)

// QueueOptions 是 asynq 消费者参数。
type QueueOptions struct {
	Enabled       bool
	Concurrency   int
	Heartbeat     bool
	RedisAddr     string
	RedisPassword string
	RedisDB       int
}

// NewQueueServer 创建 asynq 消费者并挂到生命周期；未启用时返回 nil。
func NewQueueServer(
	lc fx.Lifecycle,
	opts QueueOptions,
	sender sms.Sender,
	store storage.Storage,
	runner *Worker,
	tenantsService *tenants.Service,
	log *slog.Logger,
) *queue.Server {
	if !opts.Enabled || opts.RedisAddr == "" {
		return nil
	}

	opt := asynq.RedisClientOpt{Addr: opts.RedisAddr, Password: opts.RedisPassword, DB: opts.RedisDB}
	server := queue.NewServer(opt, opts.Concurrency)

	server.Handle(tasktype.SMSSend, func(ctx context.Context, payload []byte) error {
		var task tasktype.SMSSendPayload
		if err := json.Unmarshal(payload, &task); err != nil {
			return err
		}
		return sender.SendCode(ctx, task.Phone, task.Scene, task.Code)
	})

	server.Handle(tasktype.FileDelete, func(ctx context.Context, payload []byte) error {
		var task tasktype.FileDeletePayload
		if err := json.Unmarshal(payload, &task); err != nil {
			return err
		}
		if err := store.Delete(ctx, task.Key); err != nil && !errors.Is(err, storage.ErrNotFound) {
			return err
		}
		return nil
	})

	server.Handle(tasktype.TenantPurge, func(ctx context.Context, payload []byte) error {
		var task tasktype.TenantPurgePayload
		if err := json.Unmarshal(payload, &task); err != nil {
			return err
		}
		return runner.PurgeTenant(ctx, task.TenantID)
	})

	server.Handle(tasktype.TenantProvisionRetry, func(ctx context.Context, payload []byte) error {
		var task tasktype.TenantProvisionRetryPayload
		if err := json.Unmarshal(payload, &task); err != nil {
			return err
		}
		return tenantsService.RetryProvisionExisting(ctx, task.TenantID)
	})

	server.Handle(tasktype.DemoHeartbeat, func(ctx context.Context, _ []byte) error {
		log.InfoContext(ctx, "queue heartbeat")
		return nil
	})

	var scheduler *queue.Scheduler
	if opts.Heartbeat {
		scheduler = queue.NewScheduler(opt)
		if err := scheduler.Register("@every 1m", queue.Task{Type: tasktype.DemoHeartbeat}); err != nil {
			log.Error("register heartbeat schedule failed", "err", err)
			scheduler = nil
		}
	}

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			if err := server.Start(); err != nil {
				return err
			}
			if scheduler != nil {
				return scheduler.Start()
			}
			return nil
		},
		OnStop: func(context.Context) error {
			if scheduler != nil {
				scheduler.Shutdown()
			}
			server.Shutdown()
			return nil
		},
	})

	return server
}
