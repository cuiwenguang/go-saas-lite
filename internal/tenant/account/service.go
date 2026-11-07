package account

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/cuiwenguang/go-saas-lite/internal/security/apikey"
	"github.com/cuiwenguang/go-saas-lite/internal/security/auth"
	"github.com/cuiwenguang/go-saas-lite/internal/security/authz"
	"github.com/cuiwenguang/go-saas-lite/internal/security/identity"
	"github.com/cuiwenguang/go-saas-lite/internal/security/loginlimit"
	"github.com/cuiwenguang/go-saas-lite/internal/security/otp"
	auditlog "github.com/cuiwenguang/go-saas-lite/internal/tenant/audit"
	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

const (
	statusActive   = "active"
	refreshTTL     = 7 * 24 * time.Hour
	minPasswordLen = 8
)

// Service 是租户用户会话、资料与 API Key 自助的唯一入口。
type Service struct {
	db         tenancy.DB
	platform   tenancy.PlatformDB
	issuer     *auth.Issuer
	keys       *apikey.Service
	identities *identity.Service
	otp        *otp.Service
	loginLimit *loginlimit.Limiter
	audit      *auditlog.Service
}

// NewService 创建服务；otp / redis / audit 为 nil 时对应能力关闭。
func NewService(db tenancy.DB, platform tenancy.PlatformDB, issuer *auth.Issuer, keys *apikey.Service, identities *identity.Service, otpService *otp.Service, client *redis.Client, auditService *auditlog.Service) *Service {
	return &Service{
		db:         db,
		platform:   platform,
		issuer:     issuer,
		keys:       keys,
		identities: identities,
		otp:        otpService,
		loginLimit: loginlimit.New(client),
		audit:      auditService,
	}
}

// Login 全局登录：支持邮箱+密码、手机号+密码、手机号+验证码。
// 错误统一为 invalid credentials，避免被用来枚举账号。
func (s *Service) Login(ctx context.Context, in LoginInput) (*LoginOutput, error) {
	kind, identifier, byCode, err := in.loginIdentifier()
	if err != nil {
		return nil, err
	}

	limitKey := loginlimit.Key("tenant", strings.ToLower(identifier))
	if !s.loginLimit.Allow(ctx, limitKey) {
		return nil, apperr.New(apperr.CodeUnauthenticated, "登录失败次数过多，请稍后再试")
	}

	succeeded := false
	defer func() {
		if !succeeded {
			s.loginLimit.Fail(ctx, limitKey)
		}
	}()

	if byCode {
		if s.otp == nil {
			return nil, apperr.New(apperr.CodeUnavailable, "验证码登录未启用")
		}
		if err := s.otp.Verify(ctx, otp.SceneLogin, identifier, in.Code); err != nil {
			return nil, apperr.New(apperr.CodeUnauthenticated, "invalid credentials")
		}
	}

	hit, err := s.identities.Lookup(ctx, kind, identifier)
	if err != nil {
		return nil, apperr.New(apperr.CodeUnauthenticated, "invalid credentials")
	}

	if err := s.ensureTenantUsable(ctx, hit.TenantID); err != nil {
		return nil, err
	}

	scopeCtx := tenancy.WithScope(ctx, tenancy.Scope{
		TenantID: hit.TenantID,
		UserID:   hit.UserID,
		Actor:    tenancy.ActorTenant,
	})

	var user User
	readErr := s.db.Read(scopeCtx, func(tx tenancy.Tx) error {
		return tx.Where("id = ?", hit.UserID).First(&user)
	})
	if errors.Is(readErr, gorm.ErrRecordNotFound) {
		// 脏索引（用户已不存在）：清掉后按统一错误返回，下次对账不会重现。
		_ = s.identities.Remove(ctx, hit.TenantID, hit.UserID)

		return nil, apperr.New(apperr.CodeUnauthenticated, "invalid credentials")
	}
	if readErr != nil {
		return nil, apperr.Wrap(readErr, apperr.CodeInternal, "load user")
	}
	if user.Status != statusActive {
		return nil, apperr.New(apperr.CodeUnauthenticated, "invalid credentials")
	}
	if !byCode && bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(in.Password)) != nil {
		return nil, apperr.New(apperr.CodeUnauthenticated, "invalid credentials")
	}

	succeeded = true
	s.loginLimit.Reset(ctx, limitKey)

	return s.issueSession(scopeCtx, hit.TenantID, user)
}

// loginIdentifier 判断使用邮箱 / 手机号以及密码 / 验证码。
func (in LoginInput) loginIdentifier() (kind, value string, byCode bool, err error) {
	email := strings.ToLower(strings.TrimSpace(in.Email))
	phone := strings.TrimSpace(in.Phone)
	switch {
	case phone != "" && strings.TrimSpace(in.Code) != "":
		return identity.TypePhone, phone, true, nil
	case phone != "" && in.Password != "":
		return identity.TypePhone, phone, false, nil
	case email != "" && in.Password != "":
		return identity.TypeEmail, email, false, nil
	default:
		return "", "", false, apperr.New(apperr.CodeInvalidArgument, "credentials are required")
	}
}

// SendCode 发送验证码；短信通道未启用时返回不可用。
func (s *Service) SendCode(ctx context.Context, in SendCodeInput) error {
	if s.otp == nil {
		return apperr.New(apperr.CodeUnavailable, "验证码服务未启用")
	}

	if err := s.otp.Send(ctx, in.Scene, in.Phone); err != nil {
		switch {
		case errors.Is(err, otp.ErrTooFrequent), errors.Is(err, otp.ErrDailyLimit):
			return apperr.New(apperr.CodeQuotaExceeded, "验证码发送过于频繁")
		default:
			return apperr.Wrap(err, apperr.CodeInvalidArgument, "send code")
		}
	}

	return nil
}

// ResetPassword 用手机号验证码重置密码，并作废旧会话。
func (s *Service) ResetPassword(ctx context.Context, in ResetPasswordInput) error {
	if s.otp == nil {
		return apperr.New(apperr.CodeUnavailable, "验证码服务未启用")
	}
	if len(in.NewPassword) < minPasswordLen {
		return apperr.New(apperr.CodeInvalidArgument, "new password must be at least 8 characters")
	}
	if err := s.otp.Verify(ctx, otp.SceneReset, in.Phone, in.Code); err != nil {
		return apperr.New(apperr.CodeUnauthenticated, "invalid code")
	}

	hit, err := s.identities.Lookup(ctx, identity.TypePhone, in.Phone)
	if err != nil {
		return apperr.New(apperr.CodeUnauthenticated, "invalid credentials")
	}
	if err := s.ensureTenantUsable(ctx, hit.TenantID); err != nil {
		return err
	}

	scopeCtx := tenancy.WithScope(ctx, tenancy.Scope{
		TenantID: hit.TenantID,
		UserID:   hit.UserID,
		Actor:    tenancy.ActorTenant,
	})
	user, err := s.userByID(scopeCtx, hit.UserID)
	if err != nil {
		return err
	}
	if user.Status != statusActive {
		return apperr.New(apperr.CodeUnauthenticated, "invalid credentials")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "hash password")
	}

	return s.db.Tx(scopeCtx, func(tx tenancy.Tx) error {
		return tx.Where("id = ?", user.ID).Update(&User{}, map[string]any{
			"password_hash":        string(hash),
			"must_change_password": false,
			"token_version":        gorm.Expr("token_version + 1"),
			"refresh_jti":          "",
		})
	})
}

// ensureTenantUsable 登录前确认租户未被停用/过期（读时判定，与中间件口径一致）。
func (s *Service) ensureTenantUsable(ctx context.Context, tenantID string) error {
	var tenant Tenant
	err := s.platform.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Where("id = ?", tenantID).First(&tenant)
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperr.New(apperr.CodeTenantNotFound, "tenant not found")
	}
	if err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "load tenant")
	}

	status := tenant.Status
	if status == "active" && tenant.ExpireAt != nil && !tenant.ExpireAt.After(time.Now()) {
		status = "expired"
	}

	switch status {
	case "active":
		return nil
	case "expired":
		return apperr.New(apperr.CodeTenantExpired, "tenant has expired")
	case "pending":
		return apperr.New(apperr.CodeTenantNotReady, "tenant is not ready")
	default:
		return apperr.New(apperr.CodeTenantDisabled, "tenant is unavailable")
	}
}

// Refresh 用 refresh 令牌换一对新令牌；旧 jti 立即作废（轮换）。
func (s *Service) Refresh(ctx context.Context, raw string) (*LoginOutput, error) {
	claims, err := s.issuer.Parse(raw, auth.AudienceTenantAPI)
	if err != nil || claims.TokenType != auth.TokenRefresh || claims.Actor() != tenancy.ActorTenant || claims.TenantID == "" {
		return nil, apperr.New(apperr.CodeUnauthenticated, "invalid refresh token")
	}

	if err := s.ensureTenantUsable(ctx, claims.TenantID); err != nil {
		return nil, err
	}

	scopeCtx := tenancy.WithScope(ctx, tenancy.Scope{
		TenantID: claims.TenantID,
		UserID:   claims.Subject,
		Actor:    tenancy.ActorTenant,
	})
	user, err := s.userByID(scopeCtx, claims.Subject)
	if err != nil {
		return nil, err
	}
	if user.Status != statusActive || user.RefreshJTI == "" || user.RefreshJTI != claims.ID {
		return nil, apperr.New(apperr.CodeUnauthenticated, "refresh token revoked")
	}

	return s.issueSession(scopeCtx, claims.TenantID, *user)
}

// Logout 作废当前 refresh 令牌；令牌已失效也返回成功，登出必须幂等。
func (s *Service) Logout(ctx context.Context, raw string) error {
	if raw == "" {
		return nil
	}

	claims, err := s.issuer.Parse(raw, auth.AudienceTenantAPI)
	if err != nil || claims.TokenType != auth.TokenRefresh {
		return nil
	}

	scopeCtx := tenancy.WithScope(ctx, tenancy.Scope{TenantID: claims.TenantID, Actor: tenancy.ActorTenant})
	return s.db.Tx(scopeCtx, func(tx tenancy.Tx) error {
		return tx.Where("id = ?", claims.Subject).Update(&User{}, map[string]any{"refresh_jti": ""})
	})
}

// ChangePassword 校验旧密码后改密，并作废 refresh 令牌（需重新登录）。
func (s *Service) ChangePassword(ctx context.Context, in ChangePasswordInput) error {
	scope, err := tenancy.ScopeFrom(ctx)
	if err != nil || scope.UserID == "" {
		return apperr.New(apperr.CodeUnauthenticated, "user identity required")
	}
	if len(in.NewPassword) < minPasswordLen {
		return apperr.New(apperr.CodeInvalidArgument, "new password must be at least 8 characters")
	}

	user, err := s.userByID(ctx, scope.UserID)
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

	return s.db.Tx(ctx, func(tx tenancy.Tx) error {
		return tx.Where("id = ?", user.ID).Update(&User{}, map[string]any{
			"password_hash":        string(hash),
			"must_change_password": false,
			"token_version":        gorm.Expr("token_version + 1"),
			"refresh_jti":          "",
		})
	})
}

// Me 返回当前用户、租户与权限点集合。
func (s *Service) Me(ctx context.Context) (*MeOutput, error) {
	scope, err := tenancy.ScopeFrom(ctx)
	if err != nil || scope.UserID == "" || scope.TenantID == "" {
		return nil, apperr.New(apperr.CodeUnauthenticated, "user identity required")
	}

	user, err := s.userByID(ctx, scope.UserID)
	if err != nil {
		return nil, err
	}
	codes, perms, err := s.rolesOf(ctx, scope.UserID)
	if err != nil {
		return nil, err
	}

	var tenant Tenant
	err = s.platform.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Where("id = ?", scope.TenantID).First(&tenant)
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperr.New(apperr.CodeTenantNotFound, "tenant not found")
	}
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "load tenant")
	}

	subdomain := ""
	if tenant.Subdomain != nil {
		subdomain = *tenant.Subdomain
	}

	return &MeOutput{
		User: Profile{
			ID:          user.ID,
			Email:       user.Email,
			Phone:       derefString(user.Phone),
			DisplayName: user.DisplayName,
			Roles:       codes,
		},
		Tenant: TenantProfile{
			ID:        tenant.ID,
			Name:      tenant.Name,
			Subdomain: subdomain,
			PlanCode:  tenant.PlanCode,
		},
		Permissions: perms,
	}, nil
}

// CreateKey 创建用户 API Key。
func (s *Service) CreateKey(ctx context.Context, tenantID, userID, name string) (string, *apikey.Key, error) {
	plain, key, err := s.keys.Create(ctx, tenantID, userID, name)
	if err != nil {
		return "", nil, err
	}

	if s.audit != nil && key != nil {
		_ = s.audit.Record(ctx, auditlog.Entry{
			Category:   auditlog.CategorySecurity,
			Action:     auditlog.ActionAPIKeyCreate,
			ObjectType: "apikey",
			ObjectID:   key.ID,
			Summary:    "创建 API Key " + key.Name,
			After:      map[string]any{"name": key.Name, "prefix": key.KeyPrefix},
		})
	}

	return plain, key, nil
}

// ListKeys 列出用户的 API Key。
func (s *Service) ListKeys(ctx context.Context, tenantID, userID string) ([]apikey.Key, error) {
	return s.keys.List(ctx, tenantID, userID)
}

// RevokeKey 撤销用户的 API Key。
func (s *Service) RevokeKey(ctx context.Context, tenantID, userID, id string) error {
	if err := s.keys.Revoke(ctx, tenantID, userID, id); err != nil {
		return err
	}

	if s.audit != nil {
		_ = s.audit.Record(ctx, auditlog.Entry{
			Category:   auditlog.CategorySecurity,
			Action:     auditlog.ActionAPIKeyRevoke,
			Level:      auditlog.LevelWarn,
			ObjectType: "apikey",
			ObjectID:   id,
			Summary:    "撤销 API Key",
		})
	}

	return nil
}

// RefreshTTL 返回 refresh 令牌有效期，供 handler 设置 cookie。
func (s *Service) RefreshTTL() time.Duration {
	return refreshTTL
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}

func (s *Service) userByID(ctx context.Context, id string) (*User, error) {
	var user User
	err := s.db.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Where("id = ?", id).First(&user)
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperr.New(apperr.CodeUnauthenticated, "user not found")
	}
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "load user")
	}

	return &user, nil
}

// rolesOf 返回角色编码与展开后的权限点（`*:*` 展开为全部已登记权限）。
func (s *Service) rolesOf(ctx context.Context, userID string) ([]string, []string, error) {
	var bindings []UserRole
	var roles []Role

	if err := s.db.Read(ctx, func(tx tenancy.Tx) error {
		if err := tx.Model(&UserRole{}).Where("user_id = ?", userID).Find(&bindings); err != nil {
			return err
		}
		return tx.Model(&Role{}).Find(&roles)
	}); err != nil {
		return nil, nil, apperr.Wrap(err, apperr.CodeInternal, "load roles")
	}

	roleByID := make(map[string]Role, len(roles))
	for _, role := range roles {
		roleByID[role.ID] = role
	}

	codes := make([]string, 0, len(bindings))
	perms := make(map[string]struct{})
	wildcard := false
	for _, binding := range bindings {
		role, ok := roleByID[binding.RoleID]
		if !ok {
			continue
		}
		codes = append(codes, role.Code)

		var list []string
		if err := json.Unmarshal([]byte(role.Permissions), &list); err != nil {
			continue
		}
		for _, perm := range list {
			if perm == "*:*" {
				wildcard = true
				continue
			}
			perms[perm] = struct{}{}
		}
	}
	if wildcard {
		for _, perm := range authz.All() {
			perms[perm.String()] = struct{}{}
		}
	}

	permList := make([]string, 0, len(perms))
	for perm := range perms {
		permList = append(permList, perm)
	}
	sort.Strings(codes)
	sort.Strings(permList)

	return codes, permList, nil
}

// issueSession 签发 access + refresh，并登记新的 refresh jti（旧会话立即失效）。
func (s *Service) issueSession(ctx context.Context, tenantID string, user User) (*LoginOutput, error) {
	accessTTL := s.issuer.AccessTTL()
	access, err := s.issuer.Issue(auth.IssueOptions{
		Subject:            user.ID,
		Actor:              tenancy.ActorTenant,
		Audience:           auth.AudienceTenantAPI,
		TenantID:           tenantID,
		TokenType:          auth.TokenAccess,
		Email:              user.Email,
		MustChangePassword: user.MustChangePassword,
		TTL:                accessTTL,
	})
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "issue access token")
	}

	jti := uuid.Must(uuid.NewV7()).String()
	refresh, err := s.issuer.Issue(auth.IssueOptions{
		Subject:   user.ID,
		Actor:     tenancy.ActorTenant,
		Audience:  auth.AudienceTenantAPI,
		TenantID:  tenantID,
		TokenType: auth.TokenRefresh,
		JTI:       jti,
		TTL:       refreshTTL,
	})
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "issue refresh token")
	}

	if err := s.db.Tx(ctx, func(tx tenancy.Tx) error {
		return tx.Where("id = ?", user.ID).Update(&User{}, map[string]any{"refresh_jti": jti})
	}); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "store refresh token")
	}

	return &LoginOutput{
		AccessToken:        access,
		TokenType:          "Bearer",
		ExpiresIn:          int(accessTTL.Seconds()),
		MustChangePassword: user.MustChangePassword,
		RefreshToken:       refresh,
	}, nil
}
