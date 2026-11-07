package queue_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/cuiwenguang/go-saas-lite/pkg/queue"
	"github.com/cuiwenguang/go-saas-lite/pkg/redisx"
)

func redisOpt(t *testing.T) asynq.RedisClientOpt {
	t.Helper()

	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		addr = "127.0.0.1:6379"
	}

	client := redisx.New(redisx.Config{Addr: addr})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := redisx.Ping(ctx, client); err != nil {
		_ = client.Close()
		t.Skipf("redis unavailable at %s: %v", addr, err)
	}
	_ = client.Close()

	return asynq.RedisClientOpt{Addr: addr}
}

func TestEnqueueAndConsume(t *testing.T) {
	opt := redisOpt(t)
	taskType := "test.echo." + uuid.Must(uuid.NewV7()).String()

	server := queue.NewServer(opt, 1)
	done := make(chan string, 1)
	server.Handle(taskType, func(_ context.Context, payload []byte) error {
		done <- string(payload)
		return nil
	})
	if err := server.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(server.Shutdown)

	client := queue.NewClient(opt)
	t.Cleanup(func() { _ = client.Close() })

	ctx := context.Background()
	if err := client.Enqueue(ctx, queue.Task{
		Type:    taskType,
		Payload: map[string]string{"hello": "world"},
		ID:      "test-" + uuid.Must(uuid.NewV7()).String(),
	}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	select {
	case payload := <-done:
		if payload == "" {
			t.Fatal("empty payload")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("task was not consumed")
	}
}

func TestSchedulerRunsTask(t *testing.T) {
	opt := redisOpt(t)
	taskType := "test.schedule." + uuid.Must(uuid.NewV7()).String()

	server := queue.NewServer(opt, 1)
	done := make(chan struct{}, 1)
	server.Handle(taskType, func(context.Context, []byte) error {
		select {
		case done <- struct{}{}:
		default:
		}
		return nil
	})
	if err := server.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(server.Shutdown)

	scheduler := queue.NewScheduler(opt)
	if err := scheduler.Register("@every 1s", queue.Task{Type: taskType}); err != nil {
		t.Fatalf("register schedule: %v", err)
	}
	if err := scheduler.Start(); err != nil {
		t.Fatalf("start scheduler: %v", err)
	}
	t.Cleanup(scheduler.Shutdown)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("scheduled task did not run")
	}
}
