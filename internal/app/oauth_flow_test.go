package app_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"

	"github.com/cuiwenguang/go-saas-lite/internal/app"
	"github.com/cuiwenguang/go-saas-lite/internal/mcp"
	platformaccount "github.com/cuiwenguang/go-saas-lite/internal/platform/account"
	"github.com/cuiwenguang/go-saas-lite/internal/platform/audit"
	"github.com/cuiwenguang/go-saas-lite/internal/platform/tenants"

	"github.com/cuiwenguang/go-saas-lite/internal/routes"
	"github.com/cuiwenguang/go-saas-lite/internal/security/apikey"
	"github.com/cuiwenguang/go-saas-lite/internal/security/auth"
	"github.com/cuiwenguang/go-saas-lite/internal/security/authz"
	"github.com/cuiwenguang/go-saas-lite/internal/security/oauth"
	tenantaccount "github.com/cuiwenguang/go-saas-lite/internal/tenant/account"
	"github.com/cuiwenguang/go-saas-lite/internal/testutil"
	"github.com/cuiwenguang/go-saas-lite/pkg/redisx"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

type fullFixture struct {
	engine    *gin.Engine
	harness   *testutil.Harness
	tenantID  string
	email     string
	password  string
	issuer    *auth.Issuer
	oauth     *oauth.Handler
	platformH *platformaccount.Handler
}

func newFullFixture(t *testing.T) *fullFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)

	h := testutil.New(t)
	issuer := auth.NewIssuer("test-secret", "go-saas-lite-test")
	keys := apikey.NewService(h.Platform)
	authorizer := authz.NewService(authz.NewGormSource(h.Tenant))

	svc := tenants.NewService(h.Platform, h.Tenant, audit.NewService(h.Platform))
	hex := strings.ReplaceAll(uuid.Must(uuid.NewV7()).String(), "-", "")
	email := "owner-" + hex[len(hex)-8:] + "@oauth.test"
	password := "password123"

	created, _, err := svc.Create(t.Context(), tenants.CreateInput{
		Code:          "oauth-" + hex[len(hex)-10:],
		Name:          "OAuth Tenant",
		Timezone:      "Asia/Shanghai",
		Locale:        "zh-CN",
		AdminEmail:    email,
		AdminPassword: password,
	})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	tenantAccount := tenantaccount.NewHandler(tenantaccount.NewService(h.Tenant, issuer, keys), authorizer)
	platformH := platformaccount.NewHandler(platformaccount.NewService(h.Platform, issuer, audit.NewService(h.Platform)), platformaccount.Options{})

	var oauthHandler *oauth.Handler
	if client := testRedis(t); client != nil {
		oauthHandler = oauth.NewHandler(oauth.NewServer(h.Platform, h.Tenant, client, issuer, oauth.Config{}))
	}

	mcpHandler := mcp.NewHandler(mcp.Options{
		Provider:   mcp.NewGateway(h.Tenant),
		Authorizer: authorizer,
		Sessions:   mcp.NewMemorySessions(),
		Calls:      mcp.NewCallLog(h.Tenant),
	})

	engine := gin.New()
	public := engine.Group("/api/tenant/v1")
	routeGroups := routes.Routes{
		Public:         public,
		PlatformPublic: engine.Group("/api/platform/v1"),
		Tenant:         app.TenantChain(engine.Group("/api/tenant/v1"), issuer, app.NewTenantLookup(h.Platform)),
		Platform:       app.PlatformChain(engine.Group("/api/platform/v1"), issuer),
		MCP:            app.MCPChain(engine.Group("/api/tenant/v1/mcp"), issuer, keys, app.NewTenantLookup(h.Platform)),
	}

	tenantAccount.RegisterPublic(routeGroups.Public)
	tenantAccount.Register(routeGroups.Tenant)
	platformH.RegisterPublic(routeGroups.PlatformPublic)
	tenants.NewHandler(svc).Register(routeGroups.Platform)
	if oauthHandler != nil {
		oauthHandler.RegisterPublic(routeGroups.Public)
		oauthHandler.Register(routeGroups.Tenant)
	}
	routeGroups.MCP.POST("", gin.WrapH(mcpHandler))

	return &fullFixture{
		engine: engine, harness: h, tenantID: created.ID,
		email: email, password: password, issuer: issuer,
		oauth: oauthHandler, platformH: platformH,
	}
}

func testRedis(t *testing.T) *redis.Client {
	t.Helper()

	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		addr = "127.0.0.1:6379"
	}
	client := redisx.New(redisx.Config{Addr: addr})

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if err := redisx.Ping(ctx, client); err != nil {
		_ = client.Close()
		t.Skipf("redis unavailable at %s: %v", addr, err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func (f *fullFixture) webToken(t *testing.T) string {
	t.Helper()

	payload, _ := json.Marshal(map[string]string{"email": f.email, "password": f.password})
	req := httptest.NewRequest(http.MethodPost, "/api/tenant/v1/auth/login", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-Id", f.tenantID)

	rec := httptest.NewRecorder()
	f.engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login: %d %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return body.Data.AccessToken
}

// 平台登录 + owner 写权限。
func TestPlatformLoginAndOwnerWrite(t *testing.T) {
	f := newFullFixture(t)

	hex := strings.ReplaceAll(uuid.Must(uuid.NewV7()).String(), "-", "")
	email := "platform-" + hex[len(hex)-8:] + "@test"
	hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	if err := f.harness.Platform.Tx(t.Context(), func(tx tenancy.Tx) error {
		return tx.Create(&testutil.PlatformUser{ID: uuid.Must(uuid.NewV7()).String(), Email: email, PasswordHash: string(hash), Role: "owner", Status: "active"})
	}); err != nil {
		t.Fatalf("seed platform user: %v", err)
	}

	payload, _ := json.Marshal(map[string]string{"email": email, "password": "password123"})
	req := httptest.NewRequest(http.MethodPost, "/api/platform/v1/auth/login", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("platform login: %d %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Data.AccessToken == "" {
		t.Fatal("platform login returned empty token")
	}

	createBody, _ := json.Marshal(map[string]any{
		"code": "p-" + hex[len(hex)-10:], "name": "P", "timezone": "UTC", "locale": "zh-CN",
		"admin_email": "admin@p.test",
	})
	createReq := httptest.NewRequest(http.MethodPost, "/api/platform/v1/tenants", bytes.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+body.Data.AccessToken)
	createRec := httptest.NewRecorder()
	f.engine.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("owner create tenant: %d %s", createRec.Code, createRec.Body.String())
	}
}

// OAuth：注册 → 授权码 → PKCE 换令牌 → 调 MCP → 刷新轮换 → 旧令牌失效。
func TestOAuthAuthorizationCodeFlow(t *testing.T) {
	f := newFullFixture(t)
	if f.oauth == nil {
		t.Skip("oauth requires redis")
	}

	registerBody, _ := json.Marshal(map[string]any{
		"client_name":   "agent",
		"redirect_uris": []string{"http://127.0.0.1:9999/callback"},
	})
	regReq := httptest.NewRequest(http.MethodPost, "/api/tenant/v1/oauth/register", bytes.NewReader(registerBody))
	regReq.Header.Set("Content-Type", "application/json")
	regReq.Header.Set("X-Tenant-Id", f.tenantID)
	regRec := httptest.NewRecorder()
	f.engine.ServeHTTP(regRec, regReq)
	if regRec.Code != http.StatusCreated {
		t.Fatalf("register: %d %s", regRec.Code, regRec.Body.String())
	}

	var regBody struct {
		Data struct {
			ClientID string `json:"client_id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(regRec.Body.Bytes(), &regBody)
	clientID := regBody.Data.ClientID
	if clientID == "" {
		t.Fatal("missing client_id")
	}

	verifier := strings.Repeat("v", 48)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	authURL := "/api/tenant/v1/oauth/authorize?" + url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {"http://127.0.0.1:9999/callback"},
		"state":                 {"xyz"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"scope":                 {"mcp"},
	}.Encode()

	authReq := httptest.NewRequest(http.MethodGet, authURL, nil)
	authReq.Header.Set("Authorization", "Bearer "+f.webToken(t))
	authRec := httptest.NewRecorder()
	f.engine.ServeHTTP(authRec, authReq)
	if authRec.Code != http.StatusFound {
		t.Fatalf("authorize: %d %s", authRec.Code, authRec.Body.String())
	}

	redirect, err := url.Parse(authRec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse redirect: %v", err)
	}
	code := redirect.Query().Get("code")
	if code == "" || redirect.Query().Get("state") != "xyz" {
		t.Fatalf("unexpected redirect: %s", redirect.String())
	}

	tokenReq := httptest.NewRequest(http.MethodPost, "/api/tenant/v1/oauth/token", strings.NewReader(url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {"http://127.0.0.1:9999/callback"},
		"client_id":     {clientID},
		"code_verifier": {verifier},
	}.Encode()))
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenReq.Header.Set("X-Tenant-Id", f.tenantID)
	tokenRec := httptest.NewRecorder()
	f.engine.ServeHTTP(tokenRec, tokenReq)
	if tokenRec.Code != http.StatusOK {
		t.Fatalf("token: %d %s", tokenRec.Code, tokenRec.Body.String())
	}

	var tokens oauth.TokenOutput
	if err := json.Unmarshal(tokenRec.Body.Bytes(), &tokens); err != nil {
		t.Fatalf("decode tokens: %v", err)
	}
	if tokens.AccessToken == "" || tokens.RefreshToken == "" {
		t.Fatalf("incomplete token response: %s", tokenRec.Body.String())
	}

	// OAuth access token 可直接调 MCP。
	initReq := httptest.NewRequest(http.MethodPost, "/api/tenant/v1/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	initReq.Header.Set("Content-Type", "application/json")
	initReq.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	initRec := httptest.NewRecorder()
	f.engine.ServeHTTP(initRec, initReq)
	if initRec.Code != http.StatusOK || initRec.Header().Get(mcp.SessionHeader) == "" {
		t.Fatalf("mcp initialize via oauth: %d %s", initRec.Code, initRec.Body.String())
	}

	// 刷新后旧 refresh token 立即失效。
	refreshReq := httptest.NewRequest(http.MethodPost, "/api/tenant/v1/oauth/token", strings.NewReader(url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {tokens.RefreshToken},
	}.Encode()))
	refreshReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	refreshReq.Header.Set("X-Tenant-Id", f.tenantID)
	refreshRec := httptest.NewRecorder()
	f.engine.ServeHTTP(refreshRec, refreshReq)
	if refreshRec.Code != http.StatusOK {
		t.Fatalf("refresh: %d %s", refreshRec.Code, refreshRec.Body.String())
	}

	reuseReq := httptest.NewRequest(http.MethodPost, "/api/tenant/v1/oauth/token", strings.NewReader(url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {tokens.RefreshToken},
	}.Encode()))
	reuseReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reuseReq.Header.Set("X-Tenant-Id", f.tenantID)
	reuseRec := httptest.NewRecorder()
	f.engine.ServeHTTP(reuseRec, reuseReq)
	if reuseRec.Code == http.StatusOK {
		t.Fatal("rotated refresh token must not be reusable")
	}
}
