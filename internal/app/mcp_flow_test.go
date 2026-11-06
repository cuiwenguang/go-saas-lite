package app_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/cuiwenguang/go-saas-lite/internal/app"
	"github.com/cuiwenguang/go-saas-lite/internal/mcp"
	"github.com/cuiwenguang/go-saas-lite/internal/platform/audit"
	"github.com/cuiwenguang/go-saas-lite/internal/platform/tenants"
	"github.com/cuiwenguang/go-saas-lite/internal/routes"
	"github.com/cuiwenguang/go-saas-lite/internal/security/apikey"
	"github.com/cuiwenguang/go-saas-lite/internal/security/auth"
	"github.com/cuiwenguang/go-saas-lite/internal/security/authz"
	"github.com/cuiwenguang/go-saas-lite/internal/tenant/account"
	"github.com/cuiwenguang/go-saas-lite/internal/testutil"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

type mcpFixture struct {
	engine   *gin.Engine
	harness  *testutil.Harness
	tenantID string
	email    string
	password string
}

func newMCPFixture(t *testing.T) *mcpFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)

	h := testutil.New(t)
	issuer := auth.NewIssuer("test-secret", "go-saas-lite-test")

	svc := tenants.NewService(h.Platform, h.Tenant, audit.NewService(h.Platform))
	hex := strings.ReplaceAll(uuid.Must(uuid.NewV7()).String(), "-", "")
	email := "admin-" + hex[len(hex)-8:] + "@mcp.test"
	password := "password123"

	created, _, err := svc.Create(t.Context(), tenants.CreateInput{
		Code:          "mcp-" + hex[len(hex)-10:],
		Name:          "MCP Tenant",
		Timezone:      "Asia/Shanghai",
		Locale:        "zh-CN",
		AdminEmail:    email,
		AdminPassword: password,
	})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	keys := apikey.NewService(h.Platform)
	authorizer := authz.NewService(authz.NewGormSource(h.Tenant))
	account := account.NewHandler(account.NewService(h.Tenant, issuer, keys), authorizer)

	mcpHandler := mcp.NewHandler(mcp.Options{
		Provider:   mcp.NewGateway(h.Tenant),
		Authorizer: authorizer,
		Sessions:   mcp.NewMemorySessions(),
		Calls:      mcp.NewCallLog(h.Tenant),
	})

	engine := gin.New()
	routeGroups := routes.Routes{
		Public: engine.Group("/api/tenant/v1"),
		Tenant: app.TenantChain(engine.Group("/api/tenant/v1"), issuer, app.NewTenantLookup(h.Platform)),
		MCP:    app.MCPChain(engine.Group("/api/tenant/v1/mcp"), issuer, keys, app.NewTenantLookup(h.Platform)),
	}
	account.RegisterPublic(routeGroups.Public)
	account.Register(routeGroups.Tenant)
	routeGroups.MCP.POST("", gin.WrapH(mcpHandler))

	return &mcpFixture{engine: engine, harness: h, tenantID: created.ID, email: email, password: password}
}

func (f *mcpFixture) login(t *testing.T) string {
	t.Helper()

	payload, _ := json.Marshal(map[string]string{"email": f.email, "password": f.password})
	req := httptest.NewRequest(http.MethodPost, "/api/tenant/v1/auth/login", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-Id", f.tenantID)

	recorder := httptest.NewRecorder()
	f.engine.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", recorder.Code, recorder.Body.String())
	}

	var body struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	if body.Data.AccessToken == "" {
		t.Fatal("login returned empty access token")
	}

	return body.Data.AccessToken
}

func (f *mcpFixture) createKey(t *testing.T, token string) string {
	t.Helper()

	payload, _ := json.Marshal(map[string]string{"name": "agent"})
	req := httptest.NewRequest(http.MethodPost, "/api/tenant/v1/api-keys", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	recorder := httptest.NewRecorder()
	f.engine.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("create key failed: %d %s", recorder.Code, recorder.Body.String())
	}

	var body struct {
		Data struct {
			Plain string `json:"plain"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode key: %v", err)
	}
	if !strings.HasPrefix(body.Data.Plain, apikey.KeyPrefix) {
		t.Fatalf("unexpected key: %q", body.Data.Plain)
	}

	return body.Data.Plain
}

func (f *mcpFixture) call(t *testing.T, key, sessionID, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/tenant/v1/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	if sessionID != "" {
		req.Header.Set(mcp.SessionHeader, sessionID)
	}

	recorder := httptest.NewRecorder()
	f.engine.ServeHTTP(recorder, req)

	return recorder
}

// 完整链路：登录 → 建 API Key → MCP initialize / tools/list / tools/call。
func TestMCPFlowWithAPIKey(t *testing.T) {
	f := newMCPFixture(t)

	token := f.login(t)
	key := f.createKey(t, token)

	initResp := f.call(t, key, "", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	if initResp.Code != http.StatusOK {
		t.Fatalf("initialize: %d %s", initResp.Code, initResp.Body.String())
	}
	sessionID := initResp.Header().Get(mcp.SessionHeader)
	if sessionID == "" {
		t.Fatal("initialize must return Mcp-Session-Id")
	}

	listResp := f.call(t, key, sessionID, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	if listResp.Code != http.StatusOK {
		t.Fatalf("tools/list: %d %s", listResp.Code, listResp.Body.String())
	}
	if !strings.Contains(listResp.Body.String(), "tenant.info") {
		t.Fatalf("tools/list missing builtin tool: %s", listResp.Body.String())
	}

	callResp := f.call(t, key, sessionID, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"tenant.info","arguments":{}}}`)
	if callResp.Code != http.StatusOK {
		t.Fatalf("tools/call: %d %s", callResp.Code, callResp.Body.String())
	}
	body := callResp.Body.String()
	if !strings.Contains(body, f.tenantID) || !strings.Contains(body, `"status":"succeeded"`) && !strings.Contains(body, "content") {
		t.Fatalf("unexpected tool result: %s", body)
	}

	// 调用日志应异步落库。
	deadline := time.Now().Add(3 * time.Second)
	for {
		var count int64
		err := f.harness.Tenant.Read(testutil.Context(f.tenantID), func(tx tenancy.Tx) error {
			return tx.Raw("SELECT COUNT(*) FROM mcp_call_log").Scan(&count)
		})
		if err != nil {
			t.Fatalf("count call log: %v", err)
		}
		if count > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("mcp_call_log was not written")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// 会话必须与身份绑定：换一个 Key 复用会话应 404。
func TestMCPSessionBoundToIdentity(t *testing.T) {
	f := newMCPFixture(t)
	token := f.login(t)
	key := f.createKey(t, token)

	initResp := f.call(t, key, "", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	sessionID := initResp.Header().Get(mcp.SessionHeader)

	otherKey := f.createKey(t, token)
	resp := f.call(t, otherKey, sessionID, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("want 404 for mismatched session identity, got %d", resp.Code)
	}
}

// Web token（aud=tenant-api）不能调用 MCP。
func TestMCPRejectsWebToken(t *testing.T) {
	f := newMCPFixture(t)
	token := f.login(t)

	resp := f.call(t, token, "", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for web token on MCP, got %d", resp.Code)
	}
}
