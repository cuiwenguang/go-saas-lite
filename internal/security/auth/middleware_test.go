package auth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/cuiwenguang/go-saas-lite/internal/security/auth"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

func newGuardedEngine(t *testing.T, issuer *auth.Issuer, rule auth.Rule) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.Use(auth.Guard(issuer, rule))
	engine.GET("/probe", func(c *gin.Context) {
		scope, err := tenancy.ScopeFrom(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, scope)
	})

	return engine
}

func tenantRule() auth.Rule {
	return auth.Rule{Audience: auth.AudienceTenantAPI, Actor: tenancy.ActorTenant}
}

func doRequest(engine *gin.Engine, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	return recorder
}

func TestGuardWritesScope(t *testing.T) {
	t.Parallel()

	issuer := newIssuer()
	token, err := issuer.Issue(auth.IssueOptions{
		Subject:  "user-1",
		Actor:    tenancy.ActorTenant,
		Audience: auth.AudienceTenantAPI,
		TenantID: "tenant-1",
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	recorder := doRequest(newGuardedEngine(t, issuer, tenantRule()), token)
	if recorder.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var scope tenancy.Scope
	if err := json.Unmarshal(recorder.Body.Bytes(), &scope); err != nil {
		t.Fatalf("decode scope: %v", err)
	}
	if scope.TenantID != "tenant-1" || scope.UserID != "user-1" || scope.Actor != tenancy.ActorTenant {
		t.Fatalf("unexpected scope: %+v", scope)
	}
}

func TestGuardRejectsMissingToken(t *testing.T) {
	t.Parallel()

	recorder := doRequest(newGuardedEngine(t, newIssuer(), tenantRule()), "")
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", recorder.Code)
	}
}

// aud 不是本入口的令牌，一律 401（令牌对本资源无效）。
func TestGuardRejectsPlatformAudienceOnTenantChain(t *testing.T) {
	t.Parallel()

	issuer := newIssuer()
	token, err := issuer.Issue(auth.IssueOptions{
		Subject:  "admin-1",
		Actor:    tenancy.ActorPlatform,
		Audience: auth.AudiencePlatformAPI,
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	recorder := doRequest(newGuardedEngine(t, issuer, tenantRule()), token)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", recorder.Code)
	}
}

// aud 正确但 actor 不对，才是 actor_guard 拦下的 403。
func TestGuardRejectsMismatchedActor(t *testing.T) {
	t.Parallel()

	issuer := newIssuer()
	token, err := issuer.Issue(auth.IssueOptions{
		Subject:  "admin-1",
		Actor:    tenancy.ActorPlatform,
		Audience: auth.AudienceTenantAPI,
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	recorder := doRequest(newGuardedEngine(t, issuer, tenantRule()), token)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", recorder.Code)
	}
}

func TestGuardRejectsAgentTokenOnTenantChain(t *testing.T) {
	t.Parallel()

	issuer := newIssuer()
	token, err := issuer.Issue(auth.IssueOptions{
		Subject:  "user-1",
		Actor:    tenancy.ActorTenant,
		Audience: auth.AudienceMCP,
		TenantID: "tenant-1",
		ClientID: "client-1",
		Scope:    "mcp",
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	recorder := doRequest(newGuardedEngine(t, issuer, tenantRule()), token)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for wrong audience, got %d", recorder.Code)
	}
}

func TestGuardRejectsGarbageToken(t *testing.T) {
	t.Parallel()

	recorder := doRequest(newGuardedEngine(t, newIssuer(), tenantRule()), "not-a-jwt")
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", recorder.Code)
	}
}
