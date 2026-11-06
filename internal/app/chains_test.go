package app_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

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
	return app.TenantChain(engine.Group("/api/tenant/v1"), issuer)
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
		return app.MCPChain(engine.Group("/api/tenant/v1/mcp"), issuer, nil)
	}

	recorder := callProbe(newChainProbe(t, mount), "/api/tenant/v1/mcp/probe", token)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", recorder.Code)
	}
}
