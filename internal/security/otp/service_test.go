package otp_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/cuiwenguang/go-saas-lite/internal/security/otp"
	"github.com/cuiwenguang/go-saas-lite/pkg/redisx"
)

type captureSender struct {
	mu    sync.Mutex
	phone string
	scene string
	code  string
}

func (s *captureSender) SendCode(_ context.Context, phone, scene, code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.phone = phone
	s.scene = scene
	s.code = code
	return nil
}

func (s *captureSender) last() (string, string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.phone, s.scene, s.code
}

func redisClient(t *testing.T) *redis.Client {
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

	t.Cleanup(func() { _ = client.Close() })

	return client
}

func uniquePhone() string {
	return fmt.Sprintf("138%08d", time.Now().UnixNano()%100000000)
}

func TestSendAndVerify(t *testing.T) {
	ctx := context.Background()
	sender := &captureSender{}
	service := otp.NewService(redisClient(t), sender, otp.Options{
		TTL:            time.Minute,
		ResendInterval: time.Second,
		DailyLimit:     100,
		MaxAttempts:    3,
	})

	phone := uniquePhone()
	if err := service.Send(ctx, otp.SceneLogin, phone); err != nil {
		t.Fatalf("send: %v", err)
	}

	sentPhone, scene, code := sender.last()
	if sentPhone != phone || scene != otp.SceneLogin || len(code) != 6 {
		t.Fatalf("unexpected send: phone=%s scene=%s code=%s", sentPhone, scene, code)
	}

	wrong := "0" + code[1:]
	if code[0] == '0' {
		wrong = "1" + code[1:]
	}
	if err := service.Verify(ctx, otp.SceneLogin, phone, wrong); !errors.Is(err, otp.ErrMismatch) {
		t.Fatalf("want ErrMismatch, got %v", err)
	}
	if err := service.Verify(ctx, otp.SceneLogin, phone, code); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if err := service.Verify(ctx, otp.SceneLogin, phone, code); !errors.Is(err, otp.ErrExpired) {
		t.Fatalf("code should be single use, got %v", err)
	}
}

func TestResendCooldown(t *testing.T) {
	ctx := context.Background()
	sender := &captureSender{}
	service := otp.NewService(redisClient(t), sender, otp.Options{ResendInterval: time.Minute})

	phone := uniquePhone()
	if err := service.Send(ctx, otp.SceneSignup, phone); err != nil {
		t.Fatalf("first send: %v", err)
	}
	if err := service.Send(ctx, otp.SceneSignup, phone); !errors.Is(err, otp.ErrTooFrequent) {
		t.Fatalf("want ErrTooFrequent, got %v", err)
	}
}

func TestTooManyAttempts(t *testing.T) {
	ctx := context.Background()
	sender := &captureSender{}
	service := otp.NewService(redisClient(t), sender, otp.Options{
		TTL:            time.Minute,
		ResendInterval: time.Second,
		MaxAttempts:    2,
	})

	phone := uniquePhone()
	if err := service.Send(ctx, otp.SceneReset, phone); err != nil {
		t.Fatalf("send: %v", err)
	}
	_, _, code := sender.last()

	if err := service.Verify(ctx, otp.SceneReset, phone, "bad"); !errors.Is(err, otp.ErrMismatch) {
		t.Fatalf("first attempt: want ErrMismatch, got %v", err)
	}
	if err := service.Verify(ctx, otp.SceneReset, phone, "bad"); !errors.Is(err, otp.ErrTooManyAttempts) {
		t.Fatalf("want ErrTooManyAttempts, got %v", err)
	}
	if err := service.Verify(ctx, otp.SceneReset, phone, code); !errors.Is(err, otp.ErrExpired) {
		t.Fatalf("code should be invalidated, got %v", err)
	}
}
