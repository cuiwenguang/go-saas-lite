package account

import (
	"context"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/cuiwenguang/go-saas-lite/internal/security/apikey"
	"github.com/cuiwenguang/go-saas-lite/internal/security/auth"
	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// Service 是租户用户登录与 API Key 自助的唯一入口。
type Service struct {
	db     tenancy.DB
	issuer *auth.Issuer
	keys   *apikey.Service
}

// NewService 创建服务。
func NewService(db tenancy.DB, issuer *auth.Issuer, keys *apikey.Service) *Service {
	return &Service{db: db, issuer: issuer, keys: keys}
}

// Login 校验租户用户并签发租户令牌。
func (s *Service) Login(ctx context.Context, tenantID string, in LoginInput) (*LoginOutput, error) {
	if tenantID == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "tenant is required")
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if email == "" || in.Password == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "email and password are required")
	}

	ctx = tenancy.WithScope(ctx, tenancy.Scope{TenantID: tenantID, Actor: tenancy.ActorTenant})

	var user User
	err := s.db.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Where("email = ?", email).First(&user)
	})
	if err != nil || user.Status != "active" {
		return nil, apperr.New(apperr.CodeUnauthenticated, "invalid credentials")
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(in.Password)) != nil {
		return nil, apperr.New(apperr.CodeUnauthenticated, "invalid credentials")
	}

	ttl := s.issuer.AccessTTL()
	token, err := s.issuer.Issue(auth.IssueOptions{
		Subject:  user.ID,
		Actor:    tenancy.ActorTenant,
		Audience: auth.AudienceTenantAPI,
		TenantID: tenantID,
		TTL:      ttl,
	})
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "issue token")
	}

	return &LoginOutput{
		AccessToken:        token,
		TokenType:          "Bearer",
		ExpiresIn:          int(ttl.Seconds()),
		MustChangePassword: user.MustChangePassword,
	}, nil
}

// CreateKey 创建用户 API Key。
func (s *Service) CreateKey(ctx context.Context, tenantID, userID, name string) (string, *apikey.Key, error) {
	return s.keys.Create(ctx, tenantID, userID, name)
}

// ListKeys 列出用户的 API Key。
func (s *Service) ListKeys(ctx context.Context, tenantID, userID string) ([]apikey.Key, error) {
	return s.keys.List(ctx, tenantID, userID)
}

// RevokeKey 撤销用户的 API Key。
func (s *Service) RevokeKey(ctx context.Context, tenantID, userID, id string) error {
	return s.keys.Revoke(ctx, tenantID, userID, id)
}
