package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/cuiwenguang/go-saas-lite/internal/app"
	"github.com/cuiwenguang/go-saas-lite/internal/security/auth"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

func newChainProbe(t *testing.T, mount func(*gin.Engine, *auth.Issuer) *gin.RouterGroup) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	issuer := auth.NewIssuer("test-secret", "go-saas-lite-test")

	engine := gin.New()
	secured := mount(engine, issuer)
	secured.GET("/probe", func(c *gin.Context) {
		scope, err := tenancy.ScopeFrom(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, scope)
	})

	return engine
}

func tenantMount(engine *gin.Engine, issuer *auth.Issuer) *gin.RouterGroup {
	return app.TenantChain(engine.Group("/api/tenant/v1"), issuer, activeLookup{})
}

// activeLookup 模拟一个状态正常的租户，供只关注鉴权的用例复用。
type activeLookup struct{}

func (activeLookup) ResolveTenant(ctx context.Context, key string) (app.TenantInfo, error) {
	return app.TenantInfo{ID: key, Status: "active"}, nil
}

// expiredLookup 模拟「状态还是 active，但 expire_at 已过」的租户。
type expiredLookup struct{}

func (expiredLookup) ResolveTenant(ctx context.Context, key string) (app.TenantInfo, error) {
	past := time.Now().Add(-time.Hour)
	return app.TenantInfo{ID: key, Status: "active", ExpireAt: &past}, nil
}

// 到期读时判定：数据库里还是 active，但过期后必须按 expired 拦截。
func TestTenantChainRejectsExpiredTenant(t *testing.T) {
	t.Parallel()

	issuer := auth.NewIssuer("test-secret", "go-saas-lite-test")
	token, err := issuer.Issue(auth.IssueOptions{
		Subject:  "user-1",
		Actor:    tenancy.ActorTenant,
		Audience: auth.AudienceTenantAPI,
		TenantID: "tenant-1",
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	mount := func(engine *gin.Engine, issuer *auth.Issuer) *gin.RouterGroup {
		return app.TenantChain(engine.Group("/api/tenant/v1"), issuer, expiredLookup{})
	}

	recorder := callProbe(newChainProbe(t, mount), "/api/tenant/v1/probe", token)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "10004") {
		t.Fatalf("want tenant expired code, got %s", recorder.Body.String())
	}
}

func platformMount(engine *gin.Engine, issuer *auth.Issuer) *gin.RouterGroup {
	return app.PlatformChain(engine.Group("/api/platform/v1"), issuer)
}

func callProbe(engine *gin.Engine, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	return recorder
}

func TestTenantChainAcceptsTenantToken(t *testing.T) {
	t.Parallel()

	issuer := auth.NewIssuer("test-secret", "go-saas-lite-test")
	token, err := issuer.Issue(auth.IssueOptions{
		Subject:  "user-1",
		Actor:    tenancy.ActorTenant,
		Audience: auth.AudienceTenantAPI,
		TenantID: "tenant-1",
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	recorder := callProbe(newChainProbe(t, tenantMount), "/api/tenant/v1/probe", token)
	if recorder.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

// 平台 token 的 aud 与租户链不符，属于“令牌不适用于本入口”，返回 401。
func TestTenantChainRejectsPlatformToken(t *testing.T) {
	t.Parallel()

	issuer := auth.NewIssuer("test-secret", "go-saas-lite-test")
	token, err := issuer.Issue(auth.IssueOptions{
		Subject:  "admin-1",
		Actor:    tenancy.ActorPlatform,
		Audience: auth.AudiencePlatformAPI,
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	recorder := callProbe(newChainProbe(t, tenantMount), "/api/tenant/v1/probe", token)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", recorder.Code)
	}
}

func TestPlatformChainAcceptsPlatformToken(t *testing.T) {
	t.Parallel()

	issuer := auth.NewIssuer("test-secret", "go-saas-lite-test")
	token, err := issuer.Issue(auth.IssueOptions{
		Subject:  "admin-1",
		Actor:    tenancy.ActorPlatform,
		Audience: auth.AudiencePlatformAPI,
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	recorder := callProbe(newChainProbe(t, platformMount), "/api/platform/v1/probe", token)
	if recorder.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestPlatformChainRejectsTenantToken(t *testing.T) {
	t.Parallel()

	issuer := auth.NewIssuer("test-secret", "go-saas-lite-test")
	token, err := issuer.Issue(auth.IssueOptions{
		Subject:  "user-1",
		Actor:    tenancy.ActorTenant,
		Audience: auth.AudienceTenantAPI,
		TenantID: "tenant-1",
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	recorder := callProbe(newChainProbe(t, platformMount), "/api/platform/v1/probe", token)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", recorder.Code)
	}
}

// aud 相同但 actor 不是 tenant，由 actor_guard 拒绝。
func TestTenantChainRejectsMismatchedActor(t *testing.T) {
	t.Parallel()

	issuer := auth.NewIssuer("test-secret", "go-saas-lite-test")
	token, err := issuer.Issue(auth.IssueOptions{
		Subject:  "admin-1",
		Actor:    tenancy.ActorPlatform,
		Audience: auth.AudienceTenantAPI,
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	recorder := callProbe(newChainProbe(t, tenantMount), "/api/tenant/v1/probe", token)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", recorder.Code)
	}
}

// Web token（aud=tenant-api）不能调用 MCP 入口。
func TestMCPChainRejectsWebToken(t *testing.T) {
	t.Parallel()

	issuer := auth.NewIssuer("test-secret", "go-saas-lite-test")
	token, err := issuer.Issue(auth.IssueOptions{
		Subject:  "user-1",
		Actor:    tenancy.ActorTenant,
		Audience: auth.AudienceTenantAPI,
		TenantID: "tenant-1",
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	mount := func(engine *gin.Engine, issuer *auth.Issuer) *gin.RouterGroup {
		return app.MCPChain(engine.Group("/api/tenant/v1/mcp"), issuer, nil, activeLookup{})
	}

	recorder := callProbe(newChainProbe(t, mount), "/api/tenant/v1/mcp/probe", token)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", recorder.Code)
	}
}
