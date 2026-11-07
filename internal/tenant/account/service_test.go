package account_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"

	"github.com/cuiwenguang/go-saas-lite/internal/security/apikey"
	"github.com/cuiwenguang/go-saas-lite/internal/security/auth"
	"github.com/cuiwenguang/go-saas-lite/internal/security/identity"
	"github.com/cuiwenguang/go-saas-lite/internal/security/otp"
	"github.com/cuiwenguang/go-saas-lite/internal/tenant/account"
	"github.com/cuiwenguang/go-saas-lite/internal/testutil"
	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/redisx"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

type captureSender struct {
	mu   sync.Mutex
	code string
}

func (s *captureSender) SendCode(_ context.Context, _, _, code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.code = code
	return nil
}

func (s *captureSender) last() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.code
}

func testRedis(t *testing.T) *redis.Client {
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
	return fmt.Sprintf("139%08d", time.Now().UnixNano()%100000000)
}

func TestPhoneCodeLoginAndResetPassword(t *testing.T) {
	h := testutil.New(t)
	ctx := context.Background()
	sender := &captureSender{}
	otpService := otp.NewService(testRedis(t), sender, otp.Options{
		TTL:            time.Minute,
		ResendInterval: time.Second,
		DailyLimit:     100,
	})
	issuer := auth.NewIssuer("test-secret", "go-saas-lite-test")
	service := account.NewService(h.Tenant, h.Platform, issuer, apikey.NewService(h.Platform), h.Identity(), otpService, nil, nil)

	tenantID := h.SeedTenant(t, "account-otp")
	phone := uniquePhone()
	password := "password123"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}

	userID := uuid.Must(uuid.NewV7()).String()
	if err := h.Tenant.Tx(testutil.Context(tenantID), func(tx tenancy.Tx) error {
		return tx.Create(&account.User{
			ID:           userID,
			Phone:        &phone,
			DisplayName:  "手机用户",
			PasswordHash: string(hash),
			Status:       "active",
		})
	}); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := h.Identity().Ensure(ctx, tenantID, userID, []identity.Identifier{
		{Type: identity.TypePhone, Value: phone},
	}); err != nil {
		t.Fatalf("index phone: %v", err)
	}

	if err := otpService.Send(ctx, otp.SceneLogin, phone); err != nil {
		t.Fatalf("send login code: %v", err)
	}
	if _, err := service.Login(ctx, account.LoginInput{Phone: phone, Code: sender.last()}); err != nil {
		t.Fatalf("login by code: %v", err)
	}

	if err := otpService.Send(ctx, otp.SceneReset, phone); err != nil {
		t.Fatalf("send reset code: %v", err)
	}
	newPassword := "newpassword123"
	if err := service.ResetPassword(ctx, account.ResetPasswordInput{
		Phone:       phone,
		Code:        sender.last(),
		NewPassword: newPassword,
	}); err != nil {
		t.Fatalf("reset password: %v", err)
	}

	if _, err := service.Login(ctx, account.LoginInput{Phone: phone, Password: password}); !apperr.Is(err, apperr.CodeUnauthenticated) {
		t.Fatalf("old password should fail, got %v", err)
	}
	if _, err := service.Login(ctx, account.LoginInput{Phone: phone, Password: newPassword}); err != nil {
		t.Fatalf("new password should work: %v", err)
	}
}
