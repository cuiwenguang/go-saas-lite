package queue

import (
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

// Scheduler 是 asynq 定时任务调度器。
type Scheduler struct {
	scheduler *asynq.Scheduler
}

// NewScheduler 创建调度器。
func NewScheduler(opt asynq.RedisClientOpt) *Scheduler {
	return &Scheduler{scheduler: asynq.NewScheduler(opt, &asynq.SchedulerOpts{})}
}

// Register 注册 cron 任务。
func (s *Scheduler) Register(cron string, task Task) error {
	payload, err := json.Marshal(task.Payload)
	if err != nil {
		return fmt.Errorf("queue: marshal scheduled payload: %w", err)
	}
	if task.Payload == nil {
		payload = nil
	}

	opts := make([]asynq.Option, 0, 3)
	if task.MaxRetry > 0 {
		opts = append(opts, asynq.MaxRetry(task.MaxRetry))
	}
	if task.Timeout > 0 {
		opts = append(opts, asynq.Timeout(task.Timeout))
	}
	if task.Queue != "" {
		opts = append(opts, asynq.Queue(task.Queue))
	}

	if _, err := s.scheduler.Register(cron, asynq.NewTask(task.Type, payload), opts...); err != nil {
		return fmt.Errorf("queue: register %s: %w", task.Type, err)
	}

	return nil
}

// Start 启动调度器。
func (s *Scheduler) Start() error {
	if err := s.scheduler.Start(); err != nil {
		return fmt.Errorf("queue: start scheduler: %w", err)
	}

	return nil
}

// Shutdown 停止调度器。
func (s *Scheduler) Shutdown() {
	if s == nil || s.scheduler == nil {
		return
	}

	s.scheduler.Shutdown()
}
