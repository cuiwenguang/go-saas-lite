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
	"github.com/cuiwenguang/go-saas-lite/internal/mcp"
	mcpadmin "github.com/cuiwenguang/go-saas-lite/internal/mcp/admin"
	"github.com/cuiwenguang/go-saas-lite/internal/platform/audit"
	"github.com/cuiwenguang/go-saas-lite/internal/platform/tenants"
	"github.com/cuiwenguang/go-saas-lite/internal/routes"
	"github.com/cuiwenguang/go-saas-lite/internal/security/apikey"
	"github.com/cuiwenguang/go-saas-lite/internal/security/auth"
	"github.com/cuiwenguang/go-saas-lite/internal/security/authz"
	oauthgrant "github.com/cuiwenguang/go-saas-lite/internal/security/oauth/grant"
	"github.com/cuiwenguang/go-saas-lite/internal/tenant/account"
	"github.com/cuiwenguang/go-saas-lite/internal/tenant/members"
	"github.com/cuiwenguang/go-saas-lite/internal/tenant/stats"
	"github.com/cuiwenguang/go-saas-lite/internal/testutil"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

type seedTool struct {
	ID       string `gorm:"column:id;primaryKey"`
	TenantID string `gorm:"column:tenant_id"`
	ServerID string `gorm:"column:server_id"`
	Name     string `gorm:"column:name"`
}

func (seedTool) TableName() string { return "mcp_tools" }

// consoleFixture 覆盖租户控制台全部管理接口：成员 / 总览 / MCP / 授权。
type consoleFixture struct {
	engine   *gin.Engine
	harness  *testutil.Harness
	tenantID string
	email    string
	password string
	token    string
}

func newConsoleFixture(t *testing.T) *consoleFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)

	h := testutil.New(t)
	issuer := auth.NewIssuer("test-secret", "go-saas-lite-test")
	// 权限点由 authz 模块在装配阶段登记；手搓路由时显式登记，保证 *:* 能展开。
	if err := authz.RegisterBuiltins(); err != nil {
		t.Fatalf("register builtins: %v", err)
	}

	svc := tenants.NewService(h.Platform, h.Tenant, audit.NewService(h.Platform), h.Identity())
	hex := strings.ReplaceAll(uuid.Must(uuid.NewV7()).String(), "-", "")
	email := "console-" + hex[len(hex)-8:] + "@tenant.test"
	password := "password123"

	created, _, err := svc.Create(t.Context(), tenants.CreateInput{
		Code:          "console-" + hex[len(hex)-10:],
		Name:          "Console Tenant",
		Timezone:      "Asia/Shanghai",
		Locale:        "zh-CN",
		Subdomain:     "console-" + hex[len(hex)-10:],
		AdminEmail:    email,
		AdminPassword: password,
	})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	h.ClearMustChangePassword(t, created.ID, email)

	cipher, err := mcp.NewCipher("test-credential-key")
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}

	authorizer := authz.NewService(authz.NewGormSource(h.Tenant))
	accountHandler := account.NewHandler(account.NewService(h.Tenant, h.Platform, issuer, apikey.NewService(h.Platform), h.Identity(), nil, nil, nil), authorizer, account.Options{})

	engine := gin.New()
	lookup := app.NewTenantLookup(h.Platform)
	routeGroups := routes.Routes{
		TenantAuth: engine.Group("/api/tenant/v1"),
		Tenant:     app.TenantChain(engine.Group("/api/tenant/v1"), issuer, lookup),
	}
	accountHandler.RegisterPublic(routeGroups.TenantAuth)
	accountHandler.Register(routeGroups.Tenant)
	members.NewHandler(members.NewService(h.Tenant, h.Identity(), nil, nil), authorizer).Register(routeGroups.Tenant)
	stats.NewHandler(stats.NewService(h.Tenant, h.Platform), authorizer).Register(routeGroups.Tenant)
	mcpadmin.NewHandler(mcpadmin.NewService(h.Tenant, cipher, nil), authorizer).Register(routeGroups.Tenant)
	oauthgrant.NewHandler(oauthgrant.NewService(h.Tenant, h.Platform, nil, nil), authorizer).Register(routeGroups.Tenant)

	return &consoleFixture{
		engine:   engine,
		harness:  h,
		tenantID: created.ID,
		email:    email,
		password: password,
	}
}

type apiResponse struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

func (f *consoleFixture) call(t *testing.T, method, path, token string, body any) (int, apiResponse) {
	t.Helper()

	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(payload)
	}

	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	recorder := httptest.NewRecorder()
	f.engine.ServeHTTP(recorder, req)

	var resp apiResponse
	_ = json.Unmarshal(recorder.Body.Bytes(), &resp)

	return recorder.Code, resp
}

func (f *consoleFixture) decode(t *testing.T, path, token string, out any) {
	t.Helper()
	status, resp := f.call(t, http.MethodGet, path, token, nil)
	if status != http.StatusOK || resp.Code != 0 {
		t.Fatalf("GET %s failed: %d %s", path, status, resp.Msg)
	}
	if err := json.Unmarshal(resp.Data, out); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}

func (f *consoleFixture) login(t *testing.T) {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/tenant/v1/auth/login",
		bytes.NewReader([]byte(`{"email":"`+f.email+`","password":"`+f.password+`"}`)))
	req.Header.Set("Content-Type", "application/json")

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
	f.token = body.Data.AccessToken
}

// 全局登录：只凭邮箱就能定位到租户，不需要任何租户头。
func TestTenantLoginByEmail(t *testing.T) {
	f := newConsoleFixture(t)
	f.login(t)

	var me struct {
		Tenant struct {
			ID string `json:"id"`
		} `json:"tenant"`
	}
	f.decode(t, "/api/tenant/v1/me", f.token, &me)
	if me.Tenant.ID != f.tenantID {
		t.Fatalf("email resolved to %s, want %s", me.Tenant.ID, f.tenantID)
	}
}

func TestTenantConsoleFlow(t *testing.T) {
	f := newConsoleFixture(t)
	f.login(t)
	// /me：管理员权限点已展开，租户与套餐可见
	var me struct {
		User struct {
			Email string   `json:"email"`
			Roles []string `json:"roles"`
		} `json:"user"`
		Tenant struct {
			ID       string `json:"id"`
			PlanCode string `json:"plan_code"`
		} `json:"tenant"`
		Permissions []string `json:"permissions"`
	}
	f.decode(t, "/api/tenant/v1/me", f.token, &me)
	if me.User.Email != f.email {
		t.Fatalf("unexpected email: %s", me.User.Email)
	}
	if me.Tenant.ID != f.tenantID || me.Tenant.PlanCode == "" {
		t.Fatalf("unexpected tenant profile: %+v", me.Tenant)
	}
	if len(me.Permissions) == 0 {
		t.Fatal("admin should have expanded permissions")
	}

	// 总览：套餐与近 7 天序列
	var overview struct {
		Members struct {
			Total  int64 `json:"total"`
			Active int64 `json:"active"`
		} `json:"members"`
		Plan struct {
			Code     string `json:"code"`
			MaxUsers int64  `json:"max_users"`
		} `json:"plan"`
		CallSeries []struct {
			Date string `json:"date"`
		} `json:"call_series"`
	}
	f.decode(t, "/api/tenant/v1/stats/overview", f.token, &overview)
	if overview.Members.Total < 1 || overview.Members.Active < 1 {
		t.Fatalf("member stats wrong: %+v", overview.Members)
	}
	if overview.Plan.Code == "" {
		t.Fatal("plan code should not be empty")
	}
	if len(overview.CallSeries) != 7 {
		t.Fatalf("expected 7 series points, got %d", len(overview.CallSeries))
	}

	// 角色定义只读可见
	var roles struct {
		Items []struct {
			Code string `json:"code"`
		} `json:"items"`
	}
	f.decode(t, "/api/tenant/v1/roles", f.token, &roles)
	if len(roles.Items) < 2 {
		t.Fatalf("expected builtin roles, got %d", len(roles.Items))
	}

	// 创建成员：一次性初始密码 + 角色绑定
	memberSuffix := strings.ReplaceAll(uuid.Must(uuid.NewV7()).String(), "-", "")
	memberEmail := "member-" + memberSuffix[len(memberSuffix)-12:] + "@tenant.test"
	status, resp := f.call(t, http.MethodPost, "/api/tenant/v1/users", f.token,
		map[string]string{"email": memberEmail, "display_name": "Member One", "role": "member"})
	if status != http.StatusOK || resp.Code != 0 {
		t.Fatalf("create member failed: %d %s", status, resp.Msg)
	}
	var created struct {
		User struct {
			ID    string   `json:"id"`
			Roles []string `json:"roles"`
		} `json:"user"`
		InitialPassword string `json:"initial_password"`
	}
	if err := json.Unmarshal(resp.Data, &created); err != nil {
		t.Fatalf("decode created member: %v", err)
	}
	if created.InitialPassword == "" || created.User.ID == "" {
		t.Fatalf("create member returned incomplete payload: %+v", created)
	}
	if len(created.User.Roles) != 1 || created.User.Roles[0] != authz.RoleMember {
		t.Fatalf("unexpected roles: %v", created.User.Roles)
	}

	// 列表：包含新成员且带角色
	var list struct {
		Items []struct {
			Email              string   `json:"email"`
			Roles              []string `json:"roles"`
			MustChangePassword bool     `json:"must_change_password"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	f.decode(t, "/api/tenant/v1/users?page=1&size=20", f.token, &list)
	if list.Total < 2 {
		t.Fatalf("expected at least 2 members, got %d", list.Total)
	}
	found := false
	for _, item := range list.Items {
		if item.Email == memberEmail {
			found = item.MustChangePassword && len(item.Roles) == 1
		}
	}
	if !found {
		t.Fatalf("created member missing or malformed in list: %+v", list.Items)
	}

	// 停用成员 → 启用成员
	status, resp = f.call(t, http.MethodPatch, "/api/tenant/v1/users/"+created.User.ID, f.token,
		map[string]string{"status": "disabled"})
	if status != http.StatusOK || resp.Code != 0 {
		t.Fatalf("disable member failed: %d %s", status, resp.Msg)
	}

	// 重置密码返回一次性密码
	status, resp = f.call(t, http.MethodPost, "/api/tenant/v1/users/"+created.User.ID+"/password", f.token, nil)
	if status != http.StatusOK || resp.Code != 0 {
		t.Fatalf("reset password failed: %d %s", status, resp.Msg)
	}

	// 角色替换为管理员
	status, resp = f.call(t, http.MethodPatch, "/api/tenant/v1/users/"+created.User.ID+"/roles", f.token,
		map[string]any{"codes": []string{"admin"}})
	if status != http.StatusOK || resp.Code != 0 {
		t.Fatalf("set roles failed: %d %s", status, resp.Msg)
	}

	// 最后一个管理员保护：禁用自己应被拒绝
	var self struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	f.decode(t, "/api/tenant/v1/me", f.token, &self)
	status, resp = f.call(t, http.MethodPatch, "/api/tenant/v1/users/"+self.User.ID, f.token,
		map[string]string{"status": "disabled"})
	if status != http.StatusBadRequest || resp.Code == 0 {
		t.Fatalf("last admin guard not enforced: %d %s", status, resp.Msg)
	}

	// MCP 上游服务：创建（凭据加密）+ 列表不回显凭据
	status, resp = f.call(t, http.MethodPost, "/api/tenant/v1/mcp/servers", f.token,
		map[string]any{"name": "upstream-a", "base_url": "https://mcp.example.com/rpc", "credential": "secret-token"})
	if status != http.StatusOK || resp.Code != 0 {
		t.Fatalf("create mcp server failed: %d %s", status, resp.Msg)
	}
	var server struct {
		ID            string `json:"id"`
		HasCredential bool   `json:"has_credential"`
	}
	if err := json.Unmarshal(resp.Data, &server); err != nil {
		t.Fatalf("decode server: %v", err)
	}
	if !server.HasCredential {
		t.Fatal("credential should be stored and flagged")
	}
	if bytes.Contains(resp.Data, []byte("secret-token")) {
		t.Fatal("credential leaked in response")
	}

	// 非法 base_url 被拒
	status, resp = f.call(t, http.MethodPost, "/api/tenant/v1/mcp/servers", f.token,
		map[string]any{"name": "bad", "base_url": "ftp://example.com"})
	if status != http.StatusBadRequest {
		t.Fatalf("invalid base_url should be rejected: %d %s", status, resp.Msg)
	}

	// 工具：入库后用接口改权限点；未登记权限点被拒
	toolID := uuid.Must(uuid.NewV7()).String()
	scopeCtx := tenancy.WithScope(t.Context(), tenancy.Scope{TenantID: f.tenantID, Actor: tenancy.ActorTenant})
	if err := f.harness.Tenant.Tx(scopeCtx, func(tx tenancy.Tx) error {
		return tx.Create(&seedTool{ID: toolID, TenantID: f.tenantID, ServerID: server.ID, Name: "docs.search"})
	}); err != nil {
		t.Fatalf("seed tool: %v", err)
	}

	status, resp = f.call(t, http.MethodPatch, "/api/tenant/v1/mcp/tools/"+toolID, f.token,
		map[string]any{"required_perms": []string{"mcp:read"}})
	if status != http.StatusOK || resp.Code != 0 {
		t.Fatalf("update tool perms failed: %d %s", status, resp.Msg)
	}

	status, resp = f.call(t, http.MethodPatch, "/api/tenant/v1/mcp/tools/"+toolID, f.token,
		map[string]any{"required_perms": []string{"nope:read"}})
	if status != http.StatusBadRequest {
		t.Fatalf("unknown permission should be rejected: %d %s", status, resp.Msg)
	}

	var tools struct {
		Items []struct {
			ID            string   `json:"id"`
			RequiredPerms []string `json:"required_perms"`
		} `json:"items"`
	}
	f.decode(t, "/api/tenant/v1/mcp/tools?server_id="+server.ID, f.token, &tools)
	if len(tools.Items) != 1 || len(tools.Items[0].RequiredPerms) != 1 {
		t.Fatalf("unexpected tools payload: %+v", tools.Items)
	}

	// 调用日志分页信封
	var calls struct {
		Items []any `json:"items"`
		Page  int   `json:"page"`
		Size  int   `json:"size"`
	}
	f.decode(t, "/api/tenant/v1/mcp/calls?page=1&size=10", f.token, &calls)
	if calls.Page != 1 || calls.Size != 10 {
		t.Fatalf("unexpected paging: %+v", calls)
	}

	// 授权列表为空、撤销不存在的客户端返回 404
	var grants struct {
		Items []any `json:"items"`
	}
	f.decode(t, "/api/tenant/v1/oauth/grants", f.token, &grants)
	if len(grants.Items) != 0 {
		t.Fatalf("expected no grants, got %d", len(grants.Items))
	}
	status, _ = f.call(t, http.MethodDelete, "/api/tenant/v1/oauth/grants/no-such-client", f.token, nil)
	if status != http.StatusNotFound {
		t.Fatalf("revoke missing grant should be 404, got %d", status)
	}
}
