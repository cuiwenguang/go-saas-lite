package auth_test

import (
	"errors"
	"testing"
	"time"

	"github.com/cuiwenguang/go-saas-lite/internal/security/auth"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

func newIssuer(opts ...auth.IssuerOption) *auth.Issuer {
	return auth.NewIssuer("test-secret", "go-saas-lite-test", opts...)
}

func TestIssueAndParse(t *testing.T) {
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

	claims, err := issuer.Parse(token, auth.AudienceTenantAPI)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if claims.Subject != "user-1" || claims.TenantID != "tenant-1" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	if claims.Actor() != tenancy.ActorTenant {
		t.Fatalf("unexpected actor: %s", claims.Actor())
	}
	if claims.ID == "" {
		t.Fatal("jti should be set")
	}
}

func TestParseRejectsWrongAudience(t *testing.T) {
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

	_, err = issuer.Parse(token, auth.AudienceMCP)
	if !errors.Is(err, auth.ErrAudienceMismatch) {
		t.Fatalf("want ErrAudienceMismatch, got %v", err)
	}
}

func TestParseRejectsExpiredToken(t *testing.T) {
	t.Parallel()

	// 用过去的时钟签发，确保令牌已过期。
	past := time.Now().Add(-2 * time.Hour)
	issuer := newIssuer(auth.WithClock(func() time.Time { return past }))

	token, err := issuer.Issue(auth.IssueOptions{
		Subject:  "user-1",
		Actor:    tenancy.ActorTenant,
		Audience: auth.AudienceTenantAPI,
		TenantID: "tenant-1",
		TTL:      time.Minute,
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	_, err = issuer.Parse(token, auth.AudienceTenantAPI)
	if !errors.Is(err, auth.ErrTokenExpired) {
		t.Fatalf("want ErrTokenExpired, got %v", err)
	}
}

func TestParseRejectsTamperedToken(t *testing.T) {
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

	_, err = issuer.Parse(token+"tampered", auth.AudienceTenantAPI)
	if !errors.Is(err, auth.ErrTokenInvalid) {
		t.Fatalf("want ErrTokenInvalid, got %v", err)
	}
}

func TestParseRejectsTokenSignedWithAnotherKey(t *testing.T) {
	t.Parallel()

	other := auth.NewIssuer("another-secret", "go-saas-lite-test")
	token, err := other.Issue(auth.IssueOptions{
		Subject:  "user-1",
		Actor:    tenancy.ActorTenant,
		Audience: auth.AudienceTenantAPI,
		TenantID: "tenant-1",
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	_, err = newIssuer().Parse(token, auth.AudienceTenantAPI)
	if !errors.Is(err, auth.ErrTokenInvalid) {
		t.Fatalf("want ErrTokenInvalid, got %v", err)
	}
}

func TestIssueRequiresTenantIDForTenantActor(t *testing.T) {
	t.Parallel()

	_, err := newIssuer().Issue(auth.IssueOptions{
		Subject:  "user-1",
		Actor:    tenancy.ActorTenant,
		Audience: auth.AudienceTenantAPI,
	})
	if !errors.Is(err, auth.ErrMissingTenant) {
		t.Fatalf("want ErrMissingTenant, got %v", err)
	}
}
