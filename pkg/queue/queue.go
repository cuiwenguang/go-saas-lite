// Package queue 封装 asynq：任务定义、入队、消费与定时调度。
package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// Task 是一条待执行任务。
type Task struct {
	Type    string
	Payload any
	// ID 是幂等键；相同 ID 在 asynq 中只会存在一条任务。
	ID string
	// MaxRetry 为 0 时使用 asynq 默认值。
	MaxRetry int
	// Timeout 为 0 时使用 asynq 默认值。
	Timeout time.Duration
	// Queue 为空时进入 default 队列。
	Queue string
}

// Enqueuer 是生产者依赖的最小接口。
type Enqueuer interface {
	Enqueue(ctx context.Context, task Task) error
}

// Client 是 asynq 生产者。
type Client struct {
	client *asynq.Client
}

// NewClient 创建生产者。
func NewClient(opt asynq.RedisClientOpt) *Client {
	return &Client{client: asynq.NewClient(opt)}
}

// Enqueue 投递任务。
func (c *Client) Enqueue(ctx context.Context, task Task) error {
	payload, err := json.Marshal(task.Payload)
	if err != nil {
		return fmt.Errorf("queue: marshal payload: %w", err)
	}
	if task.Payload == nil {
		payload = nil
	}

	opts := make([]asynq.Option, 0, 4)
	if task.ID != "" {
		opts = append(opts, asynq.TaskID(task.ID))
	}
	if task.MaxRetry > 0 {
		opts = append(opts, asynq.MaxRetry(task.MaxRetry))
	}
	if task.Timeout > 0 {
		opts = append(opts, asynq.Timeout(task.Timeout))
	}
	if task.Queue != "" {
		opts = append(opts, asynq.Queue(task.Queue))
	}

	if _, err := c.client.EnqueueContext(ctx, asynq.NewTask(task.Type, payload), opts...); err != nil {
		return fmt.Errorf("queue: enqueue %s: %w", task.Type, err)
	}

	return nil
}

// Close 关闭生产者。
func (c *Client) Close() error {
	if c == nil || c.client == nil {
		return nil
	}

	return c.client.Close()
}
