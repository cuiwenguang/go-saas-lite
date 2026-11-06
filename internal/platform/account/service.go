package account

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/cuiwenguang/go-saas-lite/internal/security/auth"
	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// Service 处理平台登录与管理员初始化。
type Service struct {
	platform tenancy.PlatformDB
	issuer   *auth.Issuer
}

// NewService 创建平台认证服务。
func NewService(platform tenancy.PlatformDB, issuer *auth.Issuer) *Service {
	return &Service{platform: platform, issuer: issuer}
}

// Login 校验平台账号并签发平台令牌。
func (s *Service) Login(ctx context.Context, in LoginInput) (*LoginOutput, error) {
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if email == "" || in.Password == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "email and password are required")
	}

	var user PlatformUser
	err := s.platform.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Where("email = ?", email).First(&user)
	})
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && user.Status != "active") {
		return nil, apperr.New(apperr.CodeUnauthenticated, "invalid credentials")
	}
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "load platform user")
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(in.Password)) != nil {
		return nil, apperr.New(apperr.CodeUnauthenticated, "invalid credentials")
	}

	ttl := s.issuer.AccessTTL()
	token, err := s.issuer.Issue(auth.IssueOptions{
		Subject:  user.ID,
		Actor:    tenancy.ActorPlatform,
		Audience: auth.AudiencePlatformAPI,
		Scope:    user.Role,
		TTL:      ttl,
	})
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "issue token")
	}

	return &LoginOutput{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   int(ttl.Seconds()),
		Role:        user.Role,
	}, nil
}

// EnsureUser 幂等创建 / 更新平台管理员，供部署脚本初始化第一个账号。
func EnsureUser(ctx context.Context, platform tenancy.PlatformDB, email, password, role string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || password == "" {
		return errors.New("platform.account: email and password are required")
	}
	if role == "" {
		role = RoleOwner
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	return platform.Tx(ctx, func(tx tenancy.Tx) error {
		var existing PlatformUser
		findErr := tx.Where("email = ?", email).First(&existing)
		switch {
		case findErr == nil:
			return tx.Where("id = ?", existing.ID).Update(&PlatformUser{}, map[string]any{
				"password_hash": string(hash),
				"role":          role,
				"status":        "active",
			})
		case errors.Is(findErr, gorm.ErrRecordNotFound):
			return tx.Create(&PlatformUser{
				ID:           uuid.Must(uuid.NewV7()).String(),
				Email:        email,
				PasswordHash: string(hash),
				Role:         role,
				Status:       "active",
			})
		default:
			return findErr
		}
	})
}
