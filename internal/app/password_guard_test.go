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
	"github.com/cuiwenguang/go-saas-lite/internal/platform/audit"
	"github.com/cuiwenguang/go-saas-lite/internal/platform/tenants"
	"github.com/cuiwenguang/go-saas-lite/internal/routes"
	"github.com/cuiwenguang/go-saas-lite/internal/security/apikey"
	"github.com/cuiwenguang/go-saas-lite/internal/security/auth"
	"github.com/cuiwenguang/go-saas-lite/internal/security/authz"
	account "github.com/cuiwenguang/go-saas-lite/internal/tenant/account"
	"github.com/cuiwenguang/go-saas-lite/internal/testutil"
)

func TestPasswordChangeRequired(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := testutil.New(t)
	if err := authz.RegisterBuiltins(); err != nil {
		t.Fatalf("register builtins: %v", err)
	}

	issuer := auth.NewIssuer("test-secret", "go-saas-lite-test")
	svc := tenants.NewService(h.Platform, h.Tenant, audit.NewService(h.Platform), h.Identity())
	hex := strings.ReplaceAll(uuid.Must(uuid.NewV7()).String(), "-", "")
	email := "pwd-" + hex[len(hex)-8:] + "@test.local"
	password := "password123"

	created, _, err := svc.Create(t.Context(), tenants.CreateInput{
		Code:          "pwd-" + hex[len(hex)-10:],
		Name:          "PWD Tenant",
		Timezone:      "Asia/Shanghai",
		Locale:        "zh-CN",
		AdminEmail:    email,
		AdminPassword: password,
	})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	authorizer := authz.NewService(authz.NewGormSource(h.Tenant))
	handler := account.NewHandler(
		account.NewService(h.Tenant, h.Platform, issuer, apikey.NewService(h.Platform), h.Identity(), nil, nil, nil),
		authorizer,
		account.Options{},
	)

	engine := gin.New()
	groups := routes.Routes{
		TenantAuth: engine.Group("/api/tenant/v1"),
		Tenant:     app.TenantChain(engine.Group("/api/tenant/v1"), issuer, app.NewTenantLookup(h.Platform)),
	}
	handler.RegisterPublic(groups.TenantAuth)
	handler.Register(groups.Tenant)
	_ = created

	login := func(pass string) *httptest.ResponseRecorder {
		payload, _ := json.Marshal(map[string]string{"email": email, "password": pass})
		req := httptest.NewRequest(http.MethodPost, "/api/tenant/v1/auth/login", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		return rec
	}

	first := login(password)
	if first.Code != http.StatusOK {
		t.Fatalf("login: %d %s", first.Code, first.Body.String())
	}
	var loginBody struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	_ = json.Unmarshal(first.Body.Bytes(), &loginBody)
	token := loginBody.Data.AccessToken
	if token == "" {
		t.Fatal("empty token")
	}

	blocked := httptest.NewRequest(http.MethodGet, "/api/tenant/v1/api-keys", nil)
	blocked.Header.Set("Authorization", "Bearer "+token)
	blockedRec := httptest.NewRecorder()
	engine.ServeHTTP(blockedRec, blocked)
	if blockedRec.Code != http.StatusForbidden || !strings.Contains(blockedRec.Body.String(), "20007") {
		t.Fatalf("expected password change block, got %d %s", blockedRec.Code, blockedRec.Body.String())
	}

	newPassword := "newpassword123"
	changePayload, _ := json.Marshal(map[string]string{"old_password": password, "new_password": newPassword})
	changeReq := httptest.NewRequest(http.MethodPost, "/api/tenant/v1/auth/password", bytes.NewReader(changePayload))
	changeReq.Header.Set("Content-Type", "application/json")
	changeReq.Header.Set("Authorization", "Bearer "+token)
	changeRec := httptest.NewRecorder()
	engine.ServeHTTP(changeRec, changeReq)
	if changeRec.Code != http.StatusOK {
		t.Fatalf("change password: %d %s", changeRec.Code, changeRec.Body.String())
	}

	second := login(newPassword)
	if second.Code != http.StatusOK {
		t.Fatalf("relogin: %d %s", second.Code, second.Body.String())
	}
	_ = json.Unmarshal(second.Body.Bytes(), &loginBody)
	if loginBody.Data.AccessToken == "" {
		t.Fatal("empty token after password change")
	}

	allowed := httptest.NewRequest(http.MethodGet, "/api/tenant/v1/api-keys", nil)
	allowed.Header.Set("Authorization", "Bearer "+loginBody.Data.AccessToken)
	allowedRec := httptest.NewRecorder()
	engine.ServeHTTP(allowedRec, allowed)
	if allowedRec.Code != http.StatusOK {
		t.Fatalf("api keys after password change: %d %s", allowedRec.Code, allowedRec.Body.String())
	}
}
