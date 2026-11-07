package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// 令牌类型：access 用于访问业务接口，refresh 只用于换取新 access。
// refresh 令牌不能进业务接口，由 auth.Guard 拒绝。
const (
	TokenAccess  = "access"
	TokenRefresh = "refresh"
)

// Claims 是访问令牌载荷。权限集合不进 JWT，求值走 internal/security/authz。
type Claims struct {
	TenantID  string `json:"tenant_id,omitempty"`
	ClientID  string `json:"client_id,omitempty"`
	ActorType string `json:"actor_type"`
	Scope     string `json:"scope,omitempty"`
	TokenType string `json:"typ,omitempty"`
	// Email 是操作人邮箱，仅用于审计展示，不参与授权判定。
	Email string `json:"email,omitempty"`
	// MustChangePassword 为真时，业务接口只放行改密相关路径。
	MustChangePassword bool `json:"must_change_password,omitempty"`

	jwt.RegisteredClaims
}

// Actor 返回强类型 actor。
func (c Claims) Actor() tenancy.ActorType {
	return tenancy.ActorType(c.ActorType)
}

// Issuer 负责签发与解析令牌。
type Issuer struct {
	secret    []byte
	issuer    string
	accessTTL time.Duration
	now       func() time.Time
}

// IssuerOption 调整签发行为。
type IssuerOption func(*Issuer)

// WithAccessTTL 设置默认有效期（默认 15 分钟）。
func WithAccessTTL(ttl time.Duration) IssuerOption {
	return func(i *Issuer) {
		if ttl > 0 {
			i.accessTTL = ttl
		}
	}
}

// WithClock 便于测试控制时间。
func WithClock(now func() time.Time) IssuerOption {
	return func(i *Issuer) {
		if now != nil {
			i.now = now
		}
	}
}

// NewIssuer 用 HS256 密钥创建签发器。
func NewIssuer(secret, issuer string, opts ...IssuerOption) *Issuer {
	instance := &Issuer{
		secret:    []byte(secret),
		issuer:    issuer,
		accessTTL: 15 * time.Minute,
		now:       time.Now,
	}
	for _, opt := range opts {
		opt(instance)
	}
	return instance
}

// IssueOptions 描述一次签发。
type IssueOptions struct {
	Subject  string
	Actor    tenancy.ActorType
	Audience string
	TenantID string
	ClientID string
	Scope    string
	// TokenType 取 TokenAccess / TokenRefresh；空值按 access 处理，兼容历史令牌。
	TokenType string
	// JTI 指定载荷 id；为空时自动生成。调用方需要持久化 refresh 令牌时显式传入。
	JTI string
	// Email 是操作人邮箱，仅用于审计展示。
	Email string
	// MustChangePassword 写入 access 令牌，用于强制首次登录改密。
	MustChangePassword bool
	TTL                time.Duration
}

// AccessTTL 返回默认访问令牌有效期。
func (i *Issuer) AccessTTL() time.Duration {
	return i.accessTTL
}

// Issue 生成访问令牌。
func (i *Issuer) Issue(opts IssueOptions) (string, error) {
	if opts.Subject == "" {
		return "", fmt.Errorf("%w: subject is required", ErrTokenInvalid)
	}
	if opts.Audience == "" {
		return "", fmt.Errorf("%w: audience is required", ErrTokenInvalid)
	}
	if opts.Actor == tenancy.ActorTenant && opts.TenantID == "" {
		return "", fmt.Errorf("%w: tenant id is required for tenant actor", ErrMissingTenant)
	}

	ttl := opts.TTL
	if ttl <= 0 {
		ttl = i.accessTTL
	}

	jti := opts.JTI
	if jti == "" {
		jti = uuid.Must(uuid.NewV7()).String()
	}

	now := i.now()
	claims := Claims{
		TenantID:           opts.TenantID,
		ClientID:           opts.ClientID,
		ActorType:          string(opts.Actor),
		Scope:              opts.Scope,
		TokenType:          opts.TokenType,
		Email:              opts.Email,
		MustChangePassword: opts.MustChangePassword,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    i.issuer,
			Subject:   opts.Subject,
			Audience:  jwt.ClaimStrings{opts.Audience},
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ID:        jti,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(i.secret)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}

	return signed, nil
}

// Parse 校验签名、过期时间与 audience，并返回载荷。
// actor_type 的具体要求由调用方（中间件）比对。
func (i *Issuer) Parse(raw, audience string) (Claims, error) {
	if raw == "" {
		return Claims{}, ErrTokenMissing
	}

	claims := &Claims{}
	_, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
		return i.secret, nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
	)

	if err != nil {
		switch {
		case errors.Is(err, jwt.ErrTokenExpired):
			return Claims{}, ErrTokenExpired
		case errors.Is(err, jwt.ErrTokenInvalidAudience):
			return Claims{}, ErrAudienceMismatch
		case errors.Is(err, jwt.ErrTokenSignatureInvalid), errors.Is(err, jwt.ErrTokenMalformed):
			return Claims{}, ErrTokenInvalid
		default:
			return Claims{}, fmt.Errorf("%w: %v", ErrTokenInvalid, err)
		}
	}

	if audience != "" && !audienceMatches(claims.Audience, audience) {
		return Claims{}, ErrAudienceMismatch
	}
	if !claims.Actor().Valid() {
		return Claims{}, ErrTokenInvalid
	}

	return *claims, nil
}

func audienceMatches(audience jwt.ClaimStrings, want string) bool {
	for _, value := range audience {
		if value == want {
			return true
		}
	}
	return false
}
