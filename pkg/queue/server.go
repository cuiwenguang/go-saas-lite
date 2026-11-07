package queue

import (
	"context"
	"fmt"

	"github.com/hibiken/asynq"
)

// HandlerFunc 处理一条任务 payload。
type HandlerFunc func(ctx context.Context, payload []byte) error

// Server 是 asynq 消费者。
type Server struct {
	server *asynq.Server
	mux    *asynq.ServeMux
}

// NewServer 创建消费者；concurrency<=0 时默认 10。
func NewServer(opt asynq.RedisClientOpt, concurrency int) *Server {
	if concurrency <= 0 {
		concurrency = 10
	}

	return &Server{
		server: asynq.NewServer(opt, asynq.Config{Concurrency: concurrency}),
		mux:    asynq.NewServeMux(),
	}
}

// Handle 注册任务处理器。
func (s *Server) Handle(taskType string, fn HandlerFunc) {
	s.mux.HandleFunc(taskType, func(ctx context.Context, task *asynq.Task) error {
		return fn(ctx, task.Payload())
	})
}

// Start 启动消费者。
func (s *Server) Start() error {
	if err := s.server.Start(s.mux); err != nil {
		return fmt.Errorf("queue: start server: %w", err)
	}

	return nil
}

// Shutdown 停止消费者并等待在途任务。
func (s *Server) Shutdown() {
	if s == nil || s.server == nil {
		return
	}

	s.server.Shutdown()
}
