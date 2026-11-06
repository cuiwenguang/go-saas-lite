package authz_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/cuiwenguang/go-saas-lite/internal/security/authz"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

func TestPlatformPermissionsByRole(t *testing.T) {
	t.Parallel()

	owner := authz.PlatformPermissions(authz.PlatformRoleOwner)
	if len(owner) == 0 {
		t.Fatal("owner should have permissions")
	}

	viewer := authz.PlatformPermissions(authz.PlatformRoleViewer)
	for _, value := range viewer {
		perm, err := authz.Parse(value)
		if err != nil {
			t.Fatalf("parse %q: %v", value, err)
		}
		if !authz.PlatformAllows(authz.PlatformRoleOwner, perm) {
			t.Fatalf("owner should also hold viewer permission %q", value)
		}
	}

	if authz.PlatformAllows(authz.PlatformRoleViewer, authz.TenantCreate) {
		t.Fatal("viewer must not hold tenant:create")
	}
	if !authz.PlatformAllows(authz.PlatformRoleOwner, authz.TenantCreate) {
		t.Fatal("owner must hold tenant:create")
	}
	if got := authz.PlatformPermissions("unknown"); len(got) != 0 {
		t.Fatalf("unknown role should hold no permission, got %v", got)
	}
}

func TestPlatformRequireRejectsViewerWrite(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.Use(withPlatformRole(authz.PlatformRoleViewer))
	engine.POST("/tenants", authz.PlatformRequire(authz.TenantCreate), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/tenants", nil)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", recorder.Code)
	}
}

func TestPlatformRequireAllowsViewerRead(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.Use(withPlatformRole(authz.PlatformRoleViewer))
	engine.GET("/tenants", authz.PlatformRequire(authz.TenantRead), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/tenants", nil)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

// withPlatformRole 模拟 auth.Guard 写入 Scope 的效果。
func withPlatformRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := tenancy.WithScope(c.Request.Context(), tenancy.Scope{
			Actor: tenancy.ActorPlatform,
			Role:  role,
		})
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
