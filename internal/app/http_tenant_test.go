package app_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/cuiwenguang/go-saas-lite/internal/app"
	"github.com/cuiwenguang/go-saas-lite/internal/platform/account"
	"github.com/cuiwenguang/go-saas-lite/internal/platform/tenant"
	"github.com/cuiwenguang/go-saas-lite/internal/routes"
	"github.com/cuiwenguang/go-saas-lite/internal/security/auth"
	"github.com/cuiwenguang/go-saas-lite/internal/testutil"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

func newPlatformAPI(t *testing.T) (*gin.Engine, *auth.Issuer) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	h := testutil.New(t)
	issuer := auth.NewIssuer("test-secret", "go-saas-lite-test")

	engine := gin.New()
	routeGroups := routes.Routes{
		Platform: app.PlatformChain(engine.Group("/api/platform/v1"), issuer),
	}
	tenant.NewHandler(tenant.NewService(h.Platform, h.Tenant)).Register(routeGroups.Platform)

	return engine, issuer
}

func platformToken(t *testing.T, issuer *auth.Issuer) string {
	t.Helper()

	token, err := issuer.Issue(auth.IssueOptions{
		Subject:  "platform-admin",
		Actor:    tenancy.ActorPlatform,
		Audience: auth.AudiencePlatformAPI,
		Scope:    account.RoleOwner,
	})
	if err != nil {
		t.Fatalf("issue platform token: %v", err)
	}

	return token
}

func TestPlatformTenantAPI(t *testing.T) {
	engine, issuer := newPlatformAPI(t)
	token := platformToken(t, issuer)

	hex := strings.ReplaceAll(uuid.Must(uuid.NewV7()).String(), "-", "")
	code := "http-" + hex[len(hex)-10:]

	payload, err := json.Marshal(map[string]any{
		"code":        code,
		"name":        "HTTP Tenant",
		"timezone":    "Asia/Shanghai",
		"locale":      "zh-CN",
		"admin_email": "admin@http.test",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/platform/v1/tenants", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var created struct {
		Data struct {
			Tenant struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"tenant"`
			InitialPassword string `json:"initial_password"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if created.Data.Tenant.ID == "" || created.Data.Tenant.Status != tenant.StatusActive {
		t.Fatalf("unexpected tenant payload: %+v", created.Data)
	}
	if created.Data.InitialPassword == "" {
		t.Fatal("initial password must be returned once")
	}

	// 列表接口按状态过滤。
	listReq := httptest.NewRequest(http.MethodGet, "/api/platform/v1/tenants?status=active", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listRecorder := httptest.NewRecorder()
	engine.ServeHTTP(listRecorder, listReq)

	if listRecorder.Code != http.StatusOK {
		t.Fatalf("list want 200, got %d", listRecorder.Code)
	}

	// 没有令牌必须拒绝。
	anonReq := httptest.NewRequest(http.MethodGet, "/api/platform/v1/tenants", nil)
	anonRecorder := httptest.NewRecorder()
	engine.ServeHTTP(anonRecorder, anonReq)

	if anonRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous request want 401, got %d", anonRecorder.Code)
	}
}
