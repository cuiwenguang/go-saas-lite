package account

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/cuiwenguang/go-saas-lite/internal/platform/audit"
	"github.com/cuiwenguang/go-saas-lite/internal/security/auth"
	"github.com/cuiwenguang/go-saas-lite/internal/security/authz"
	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

const (
	// statusActive 是 platform_users.status 的有效值。
	statusActive = "active"
	// refreshTTL 是平台 refresh 令牌有效期（readme 2.2：7 天）。
	refreshTTL = 7 * 24 * time.Hour
	// minPasswordLen 是密码最小长度。
	minPasswordLen = 8
)

// Service 处理平台登录与管理员初始化。
type Service struct {
	platform tenancy.PlatformDB
	issuer   *auth.Issuer
	audit    *audit.Service
}

// NewService 创建平台认证服务。
func NewService(platform tenancy.PlatformDB, issuer *auth.Issuer, auditService *audit.Service) *Service {
	return &Service{platform: platform, issuer: issuer, audit: auditService}
}

// Login 校验平台账号并签发 access / refresh 令牌。
func (s *Service) Login(ctx context.Context, in LoginInput) (*LoginOutput, error) {
	user, err := s.authenticate(ctx, in)
	if err != nil {
		s.recordAudit(ctx, audit.Entry{
			Category:   audit.CategoryAuth,
			Action:     audit.ActionLoginFailed,
			Level:      audit.LevelWarn,
			Result:     audit.ResultFailed,
			ActorType:  audit.ActorAdmin,
			ActorEmail: strings.ToLower(strings.TrimSpace(in.Email)),
			Summary:    "平台登录失败：" + in.Email,
		})
		return nil, err
	}

	out, err := s.issueSession(ctx, *user)
	if err != nil {
		return nil, err
	}

	s.recordAudit(ctx, audit.Entry{
		Category:   audit.CategoryAuth,
		Action:     audit.ActionLogin,
		ActorType:  audit.ActorAdmin,
		ActorID:    user.ID,
		ActorEmail: user.Email,
		Summary:    "平台管理员登录",
	})

	return out, nil
}

// Refresh 用 refresh 令牌换一对新令牌；旧 jti 立即作废（轮换）。
func (s *Service) Refresh(ctx context.Context, raw string) (*LoginOutput, error) {
	claims, err := s.issuer.Parse(raw, auth.AudiencePlatformAPI)
	if err != nil || claims.TokenType != auth.TokenRefresh || claims.Actor() != tenancy.ActorPlatform {
		return nil, apperr.New(apperr.CodeUnauthenticated, "invalid refresh token")
	}

	user, err := s.userByID(ctx, claims.Subject)
	if err != nil {
		return nil, err
	}
	if user.Status != statusActive || user.RefreshJTI == "" || user.RefreshJTI != claims.ID {
		return nil, apperr.New(apperr.CodeUnauthenticated, "refresh token revoked")
	}

	return s.issueSession(ctx, *user)
}

// Logout 作废当前 refresh 令牌；令牌已失效也返回成功，登出必须幂等。
func (s *Service) Logout(ctx context.Context, raw string) error {
	if raw == "" {
		return nil
	}

	claims, err := s.issuer.Parse(raw, auth.AudiencePlatformAPI)
	if err != nil || claims.TokenType != auth.TokenRefresh {
		return nil
	}

	return s.platform.Tx(ctx, func(tx tenancy.Tx) error {
		return tx.Where("id = ?", claims.Subject).Update(&PlatformUser{}, map[string]any{"refresh_jti": ""})
	})
}

// ChangePassword 校验旧密码后改密，并作废 refresh 令牌（需重新登录）。
func (s *Service) ChangePassword(ctx context.Context, userID string, in ChangePasswordInput) error {
	if len(in.NewPassword) < minPasswordLen {
		return apperr.New(apperr.CodeInvalidArgument, "new password must be at least 8 characters")
	}

	user, err := s.userByID(ctx, userID)
	if err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(in.OldPassword)) != nil {
		return apperr.New(apperr.CodeUnauthenticated, "old password mismatch")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "hash password")
	}

	if err := s.platform.Tx(ctx, func(tx tenancy.Tx) error {
		return tx.Where("id = ?", user.ID).Update(&PlatformUser{}, map[string]any{
			"password_hash": string(hash),
			"refresh_jti":   "",
		})
	}); err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "update password")
	}

	s.recordAudit(ctx, audit.Entry{
		Category: audit.CategoryAuth,
		Action:   audit.ActionPasswordChange,
		Level:    audit.LevelWarn,
		Summary:  "修改平台账号密码",
	})

	return nil
}

// Me 返回当前平台账号、角色与权限点。
func (s *Service) Me(ctx context.Context, userID string) (*MeOutput, error) {
	user, err := s.userByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &MeOutput{
		User: PlatformProfile{
			ID:    user.ID,
			Email: user.Email,
			Role:  user.Role,
		},
		Role:        user.Role,
		Permissions: authz.PlatformPermissions(user.Role),
	}, nil
}

// RefreshTTL 返回 refresh 令牌有效期，供 handler 设置 cookie。
func (s *Service) RefreshTTL() time.Duration {
	return refreshTTL
}

// recordAudit 写审计日志；失败不能阻断主流程。
func (s *Service) recordAudit(ctx context.Context, entry audit.Entry) {
	if s.audit == nil {
		return
	}
	_ = s.audit.Record(ctx, entry)
}

func (s *Service) authenticate(ctx context.Context, in LoginInput) (*PlatformUser, error) {
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if email == "" || in.Password == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "email and password are required")
	}

	var user PlatformUser
	err := s.platform.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Where("email = ?", email).First(&user)
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperr.New(apperr.CodeUnauthenticated, "invalid credentials")
	}
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "load platform user")
	}
	if user.Status != statusActive {
		return nil, apperr.New(apperr.CodeUnauthenticated, "invalid credentials")
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(in.Password)) != nil {
		return nil, apperr.New(apperr.CodeUnauthenticated, "invalid credentials")
	}

	return &user, nil
}

func (s *Service) userByID(ctx context.Context, id string) (*PlatformUser, error) {
	var user PlatformUser
	err := s.platform.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Where("id = ?", id).First(&user)
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperr.New(apperr.CodeUnauthenticated, "platform user not found")
	}
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "load platform user")
	}

	return &user, nil
}

// issueSession 签发 access + refresh，并登记新的 refresh jti（旧会话立即失效）。
func (s *Service) issueSession(ctx context.Context, user PlatformUser) (*LoginOutput, error) {
	accessTTL := s.issuer.AccessTTL()
	access, err := s.issuer.Issue(auth.IssueOptions{
		Subject:   user.ID,
		Actor:     tenancy.ActorPlatform,
		Audience:  auth.AudiencePlatformAPI,
		Scope:     user.Role,
		TokenType: auth.TokenAccess,
		Email:     user.Email,
		TTL:       accessTTL,
	})
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "issue access token")
	}

	jti := uuid.Must(uuid.NewV7()).String()
	refresh, err := s.issuer.Issue(auth.IssueOptions{
		Subject:   user.ID,
		Actor:     tenancy.ActorPlatform,
		Audience:  auth.AudiencePlatformAPI,
		Scope:     user.Role,
		TokenType: auth.TokenRefresh,
		JTI:       jti,
		TTL:       refreshTTL,
	})
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "issue refresh token")
	}

	if err := s.platform.Tx(ctx, func(tx tenancy.Tx) error {
		return tx.Where("id = ?", user.ID).Update(&PlatformUser{}, map[string]any{"refresh_jti": jti})
	}); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "store refresh token")
	}

	return &LoginOutput{
		AccessToken:  access,
		TokenType:    "Bearer",
		ExpiresIn:    int(accessTTL.Seconds()),
		Role:         user.Role,
		RefreshToken: refresh,
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
