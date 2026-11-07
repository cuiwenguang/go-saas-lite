package portal_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/cuiwenguang/go-saas-lite/internal/platform/audit"
	"github.com/cuiwenguang/go-saas-lite/internal/platform/tenants"
	"github.com/cuiwenguang/go-saas-lite/internal/portal"
	"github.com/cuiwenguang/go-saas-lite/internal/security/otp"
	"github.com/cuiwenguang/go-saas-lite/internal/testutil"
	"github.com/cuiwenguang/go-saas-lite/pkg/redisx"
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
	return fmt.Sprintf("137%08d", time.Now().UnixNano()%100000000)
}

func newPortal(t *testing.T, autoApprove bool) (*portal.Service, *captureSender, *tenants.Service) {
	t.Helper()

	h := testutil.New(t)
	sender := &captureSender{}
	otpService := otp.NewService(testRedis(t), sender, otp.Options{
		TTL:            time.Minute,
		ResendInterval: time.Second,
		DailyLimit:     100,
	})
	tenantService := tenants.NewService(h.Platform, h.Tenant, audit.NewService(h.Platform), h.Identity())

	return portal.NewService(tenantService, otpService, h.Platform, portal.Options{
		ConsoleURL:  "http://console.test",
		AutoApprove: autoApprove,
	}), sender, tenantService
}

func TestSignupAutoApprove(t *testing.T) {
	service, sender, tenantService := newPortal(t, true)
	ctx := context.Background()
	phone := uniquePhone()

	if err := service.SendSignupCode(ctx, phone); err != nil {
		t.Fatalf("send code: %v", err)
	}

	result, err := service.Signup(ctx, portal.SignupInput{
		CompanyName: "星河科技",
		Phone:       phone,
		Code:        sender.last(),
		Password:    "password123",
		AcceptTerms: true,
	})
	if err != nil {
		t.Fatalf("signup: %v", err)
	}
	if !strings.HasPrefix(result.Code, "t-") {
		t.Fatalf("expected fallback tenant code, got %s", result.Code)
	}
	if result.Pending {
		t.Fatal("auto approve should not be pending")
	}

	tenant, err := tenantService.Get(ctx, result.TenantID)
	if err != nil {
		t.Fatalf("get tenant: %v", err)
	}
	if tenant.Status != tenants.StatusActive {
		t.Fatalf("want active, got %s", tenant.Status)
	}
}

func TestSignupPendingReview(t *testing.T) {
	service, sender, tenantService := newPortal(t, false)
	ctx := context.Background()
	phone := uniquePhone()

	if err := service.SendSignupCode(ctx, phone); err != nil {
		t.Fatalf("send code: %v", err)
	}

	result, err := service.Signup(ctx, portal.SignupInput{
		CompanyName: "Acme Labs",
		Phone:       phone,
		Code:        sender.last(),
		Password:    "password123",
		AcceptTerms: true,
	})
	if err != nil {
		t.Fatalf("signup: %v", err)
	}
	if !result.Pending {
		t.Fatal("expected pending result")
	}

	tenant, err := tenantService.Get(ctx, result.TenantID)
	if err != nil {
		t.Fatalf("get tenant: %v", err)
	}
	if tenant.Status != tenants.StatusPending {
		t.Fatalf("want pending, got %s", tenant.Status)
	}
	if tenant.ProvisionStatus != tenants.ProvisionSucceeded {
		t.Fatalf("provision should succeed, got %s", tenant.ProvisionStatus)
	}
}

func TestSignupRejectsWrongCode(t *testing.T) {
	service, _, _ := newPortal(t, true)
	ctx := context.Background()
	phone := uniquePhone()

	if err := service.SendSignupCode(ctx, phone); err != nil {
		t.Fatalf("send code: %v", err)
	}

	_, err := service.Signup(ctx, portal.SignupInput{
		CompanyName: "Acme Labs",
		Phone:       phone,
		Code:        "000000",
		Password:    "password123",
		AcceptTerms: true,
	})
	if err == nil {
		t.Fatal("wrong code should be rejected")
	}
}

func TestLandingAndCSS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, err := portal.NewHandler(portal.NewService(nil, nil, nil, portal.Options{}), portal.Options{ConsoleURL: "http://console.test"}, nil)
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}

	engine := gin.New()
	handler.Register(engine.Group(""))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "把租户、权限、配额和 MCP 一次装好") {
		t.Fatalf("landing page: %d %s", rec.Code, rec.Body.String())
	}

	css := httptest.NewRecorder()
	engine.ServeHTTP(css, httptest.NewRequest(http.MethodGet, "/portal.css", nil))
	if css.Code != http.StatusOK || css.Body.Len() == 0 {
		t.Fatalf("portal css: %d", css.Code)
	}

	login := httptest.NewRecorder()
	engine.ServeHTTP(login, httptest.NewRequest(http.MethodGet, "/login", nil))
	if login.Code != http.StatusFound {
		t.Fatalf("login should redirect, got %d", login.Code)
	}
}
