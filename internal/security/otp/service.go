// Package otp 负责短信验证码的生成、发送、校验与频率限制。
package otp

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/cuiwenguang/go-saas-lite/internal/security/identity"
	"github.com/cuiwenguang/go-saas-lite/pkg/sms"
)

// 验证码场景。
const (
	SceneSignup = "signup"
	SceneLogin  = "login"
	SceneReset  = "reset"
)

var (
	// ErrUnavailable 表示验证码服务未启用。
	ErrUnavailable = errors.New("otp: service unavailable")
	// ErrTooFrequent 表示发送过于频繁。
	ErrTooFrequent = errors.New("otp: too frequent")
	// ErrDailyLimit 表示达到当日发送上限。
	ErrDailyLimit = errors.New("otp: daily limit reached")
	// ErrExpired 表示验证码不存在或已过期。
	ErrExpired = errors.New("otp: code expired")
	// ErrMismatch 表示验证码不匹配。
	ErrMismatch = errors.New("otp: code mismatch")
	// ErrTooManyAttempts 表示校验次数过多。
	ErrTooManyAttempts = errors.New("otp: too many attempts")
)

// Options 是验证码策略。
type Options struct {
	TTL            time.Duration
	ResendInterval time.Duration
	DailyLimit     int
	MaxAttempts    int
}

func (o Options) withDefaults() Options {
	if o.TTL <= 0 {
		o.TTL = 5 * time.Minute
	}
	if o.ResendInterval <= 0 {
		o.ResendInterval = 60 * time.Second
	}
	if o.DailyLimit <= 0 {
		o.DailyLimit = 10
	}
	if o.MaxAttempts <= 0 {
		o.MaxAttempts = 5
	}
	return o
}

// Service 是验证码服务。
type Service struct {
	client redis.UniversalClient
	sender sms.Sender
	opts   Options
	now    func() time.Time
}

// NewService 创建验证码服务。
func NewService(client redis.UniversalClient, sender sms.Sender, opts Options) *Service {
	return &Service{client: client, sender: sender, opts: opts.withDefaults(), now: time.Now}
}

// Send 生成并发送验证码。
func (s *Service) Send(ctx context.Context, scene, rawPhone string) error {
	phone, err := identity.Normalize(identity.TypePhone, rawPhone)
	if err != nil {
		return err
	}
	if !validScene(scene) {
		return fmt.Errorf("otp: unsupported scene %q", scene)
	}

	cooldownKey := fmt.Sprintf("otp:cooldown:%s:%s", scene, phone)
	allowed, err := s.client.SetNX(ctx, cooldownKey, "1", s.opts.ResendInterval).Result()
	if err != nil {
		return fmt.Errorf("otp: reserve cooldown: %w", err)
	}
	if !allowed {
		return ErrTooFrequent
	}

	dailyKey := fmt.Sprintf("otp:daily:%s:%s", phone, s.now().Format("2006-01-02"))
	count, err := s.client.Incr(ctx, dailyKey).Result()
	if err != nil {
		_ = s.client.Del(ctx, cooldownKey).Err()
		return fmt.Errorf("otp: incr daily count: %w", err)
	}
	if count == 1 {
		_ = s.client.Expire(ctx, dailyKey, 24*time.Hour).Err()
	}
	if count > int64(s.opts.DailyLimit) {
		return ErrDailyLimit
	}

	code, err := randomCode()
	if err != nil {
		_ = s.client.Del(ctx, cooldownKey).Err()
		return fmt.Errorf("otp: generate code: %w", err)
	}

	codeKey := codeKey(scene, phone)
	if err := s.client.Set(ctx, codeKey, code, s.opts.TTL).Err(); err != nil {
		_ = s.client.Del(ctx, cooldownKey).Err()
		return fmt.Errorf("otp: store code: %w", err)
	}
	_ = s.client.Del(ctx, attemptsKey(scene, phone)).Err()

	if err := s.sender.SendCode(ctx, phone, scene, code); err != nil {
		// 发送失败允许立即重试。
		_ = s.client.Del(ctx, codeKey, cooldownKey).Err()
		return fmt.Errorf("otp: send code: %w", err)
	}

	return nil
}

// Verify 校验并消费验证码；成功后立即失效。
func (s *Service) Verify(ctx context.Context, scene, rawPhone, rawCode string) error {
	phone, err := identity.Normalize(identity.TypePhone, rawPhone)
	if err != nil {
		return ErrMismatch
	}
	if !validScene(scene) {
		return ErrMismatch
	}

	code := strings.TrimSpace(rawCode)
	if code == "" {
		return ErrMismatch
	}

	key := codeKey(scene, phone)
	expected, err := s.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return ErrExpired
	}
	if err != nil {
		return fmt.Errorf("otp: read code: %w", err)
	}

	if subtle.ConstantTimeCompare([]byte(expected), []byte(code)) != 1 {
		attempts, incrErr := s.client.Incr(ctx, attemptsKey(scene, phone)).Result()
		if incrErr == nil {
			_ = s.client.Expire(ctx, attemptsKey(scene, phone), s.opts.TTL).Err()
			if attempts >= int64(s.opts.MaxAttempts) {
				_ = s.client.Del(ctx, key).Err()
				return ErrTooManyAttempts
			}
		}

		return ErrMismatch
	}

	_ = s.client.Del(ctx, key).Err()
	_ = s.client.Del(ctx, attemptsKey(scene, phone)).Err()

	return nil
}

func validScene(scene string) bool {
	switch scene {
	case SceneSignup, SceneLogin, SceneReset:
		return true
	default:
		return false
	}
}

func codeKey(scene, phone string) string {
	return fmt.Sprintf("otp:code:%s:%s", scene, phone)
}

func attemptsKey(scene, phone string) string {
	return fmt.Sprintf("otp:attempts:%s:%s", scene, phone)
}

func randomCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%06d", n.Int64()), nil
}
