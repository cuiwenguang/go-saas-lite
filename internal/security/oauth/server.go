// Package oauth 实现 Agent 接入所需的 OAuth 2.1（授权码 + PKCE + 刷新轮换）。
package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/cuiwenguang/go-saas-lite/internal/security/auth"
	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

const (
	codeKeyPrefix    = "oauth:code:"
	refreshKeyPrefix = "oauth:refresh:"

	// ScopeMCP 是 Agent 唯一允许的 scope。
	ScopeMCP = "mcp"
)

// ErrInvalidGrant 表示授权码 / 刷新令牌不合法。
var ErrInvalidGrant = errors.New("oauth: invalid grant")

// Config 是令牌有效期。
type Config struct {
	CodeTTL    time.Duration
	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

func (c Config) withDefaults() Config {
	if c.CodeTTL <= 0 {
		c.CodeTTL = 10 * time.Minute
	}
	if c.AccessTTL <= 0 {
		c.AccessTTL = time.Hour
	}
	if c.RefreshTTL <= 0 {
		c.RefreshTTL = 7 * 24 * time.Hour
	}
	return c
}

// Server 是授权服务器。
type Server struct {
	clients tenancy.PlatformDB
	grants  tenancy.DB
	redis   redis.UniversalClient
	issuer  *auth.Issuer
	cfg     Config
	now     func() time.Time
}

// NewServer 创建授权服务器。
func NewServer(clients tenancy.PlatformDB, grants tenancy.DB, client redis.UniversalClient, issuer *auth.Issuer, cfg Config) *Server {
	return &Server{
		clients: clients,
		grants:  grants,
		redis:   client,
		issuer:  issuer,
		cfg:     cfg.withDefaults(),
		now:     time.Now,
	}
}

// Register 动态注册客户端（RFC 7591）；只允许 HTTPS 与本地回环。
func (s *Server) Register(ctx context.Context, tenantID string, req RegisterInput) (*RegisterOutput, error) {
	if tenantID == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "tenant is required")
	}
	if len(req.RedirectURIs) == 0 {
		return nil, apperr.New(apperr.CodeInvalidArgument, "redirect_uris is required")
	}
	for _, uri := range req.RedirectURIs {
		if err := validateRedirectURI(uri); err != nil {
			return nil, err
		}
	}

	raw, err := json.Marshal(req.RedirectURIs)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "marshal redirect uris")
	}

	row := &OAuthClient{
		ClientID:     "client_" + uuid.Must(uuid.NewV7()).String(),
		TenantID:     tenantID,
		ClientName:   req.ClientName,
		RedirectURIs: string(raw),
		GrantTypes:   `["authorization_code","refresh_token"]`,
	}
	if err := s.clients.Tx(ctx, func(tx tenancy.Tx) error {
		return tx.Create(row)
	}); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "register client")
	}

	return &RegisterOutput{ClientID: row.ClientID, ClientName: row.ClientName, RedirectURIs: req.RedirectURIs}, nil
}

type codePayload struct {
	TenantID      string `json:"tenant_id"`
	UserID        string `json:"user_id"`
	ClientID      string `json:"client_id"`
	RedirectURI   string `json:"redirect_uri"`
	Scope         string `json:"scope"`
	CodeChallenge string `json:"code_challenge"`
}

// Authorize 校验客户端与回调地址，生成授权码并返回跳转地址。
func (s *Server) Authorize(ctx context.Context, scope tenancy.Scope, req AuthorizeInput) (string, error) {
	if scope.TenantID == "" || scope.UserID == "" {
		return "", apperr.New(apperr.CodeUnauthenticated, "user session required")
	}
	if req.State == "" {
		return "", apperr.New(apperr.CodeInvalidArgument, "state is required")
	}
	if req.CodeChallengeMethod != "S256" {
		return "", apperr.New(apperr.CodeInvalidArgument, "code_challenge_method must be S256")
	}
	if len(req.CodeChallenge) < 43 {
		return "", apperr.New(apperr.CodeInvalidArgument, "invalid code_challenge")
	}

	client, err := s.loadClient(ctx, scope.TenantID, req.ClientID)
	if err != nil {
		return "", err
	}
	if !contains(client.URIs, req.RedirectURI) {
		return "", apperr.New(apperr.CodeInvalidArgument, "redirect_uri does not match registration")
	}

	code := randomToken(32)
	payload := codePayload{
		TenantID:      scope.TenantID,
		UserID:        scope.UserID,
		ClientID:      req.ClientID,
		RedirectURI:   req.RedirectURI,
		Scope:         scopeOrDefault(req.Scope),
		CodeChallenge: req.CodeChallenge,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", apperr.Wrap(err, apperr.CodeInternal, "marshal code")
	}
	if err := s.redis.Set(ctx, codeKeyPrefix+code, raw, s.cfg.CodeTTL).Err(); err != nil {
		return "", apperr.Wrap(err, apperr.CodeInternal, "store code")
	}

	redirect, err := url.Parse(req.RedirectURI)
	if err != nil {
		return "", apperr.New(apperr.CodeInvalidArgument, "invalid redirect_uri")
	}
	query := redirect.Query()
	query.Set("code", code)
	query.Set("state", req.State)
	redirect.RawQuery = query.Encode()

	return redirect.String(), nil
}

type refreshPayload struct {
	TenantID string `json:"tenant_id"`
	UserID   string `json:"user_id"`
	ClientID string `json:"client_id"`
	Scope    string `json:"scope"`
	Family   string `json:"family"`
}

// Exchange 用授权码换令牌，校验 PKCE。
func (s *Server) Exchange(ctx context.Context, req TokenInput) (*TokenOutput, error) {
	if req.GrantType != "authorization_code" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "unsupported grant_type")
	}

	raw, err := s.take(ctx, codeKeyPrefix+req.Code)
	if errors.Is(err, redis.Nil) {
		return nil, apperr.New(apperr.CodeInvalidToken, "authorization code is invalid or used")
	}
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "read code")
	}

	var payload codePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "parse code")
	}
	if payload.ClientID != req.ClientID || payload.RedirectURI != req.RedirectURI {
		return nil, apperr.New(apperr.CodeInvalidToken, "client or redirect_uri mismatch")
	}
	if !verifyPKCE(req.CodeVerifier, payload.CodeChallenge) {
		return nil, apperr.New(apperr.CodeInvalidToken, "code_verifier mismatch")
	}

	if err := s.upsertGrant(ctx, payload); err != nil {
		return nil, err
	}

	return s.issue(ctx, payload.TenantID, payload.UserID, payload.ClientID, payload.Scope, randomToken(16))
}

// Refresh 轮换刷新令牌；旧令牌立即失效。
func (s *Server) Refresh(ctx context.Context, tenantID, refreshToken string) (*TokenOutput, error) {
	fd := refreshKeyPrefix + hashToken(refreshToken)
	raw, err := s.take(ctx, fd)
	if errors.Is(err, redis.Nil) {
		return nil, apperr.New(apperr.CodeInvalidToken, "refresh token is invalid or used")
	}
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "read refresh token")
	}

	var payload refreshPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "parse refresh token")
	}
	if payload.TenantID != tenantID {
		return nil, apperr.New(apperr.CodeInvalidToken, "refresh token tenant mismatch")
	}

	var grant OAuthGrant
	findErr := s.grants.Read(withScope(ctx, payload), func(tx tenancy.Tx) error {
		return tx.Where("user_id = ? AND client_id = ?", payload.UserID, payload.ClientID).First(&grant)
	})
	if findErr != nil || grant.RevokedAt != nil {
		return nil, apperr.New(apperr.CodeInvalidToken, "grant revoked")
	}

	return s.issue(ctx, payload.TenantID, payload.UserID, payload.ClientID, payload.Scope, payload.Family)
}

// Revoke 撤销刷新令牌并作废授权记录。
func (s *Server) Revoke(ctx context.Context, tenantID, refreshToken string) error {
	key := refreshKeyPrefix + hashToken(refreshToken)

	raw, err := s.redis.Get(ctx, key).Bytes()
	if err != nil && !errors.Is(err, redis.Nil) {
		return apperr.Wrap(err, apperr.CodeInternal, "read refresh token")
	}

	if err == nil {
		var payload refreshPayload
		if json.Unmarshal(raw, &payload) == nil && payload.TenantID == tenantID {
			_ = s.grants.Tx(withScope(ctx, payload), func(tx tenancy.Tx) error {
				return tx.Where("user_id = ? AND client_id = ?", payload.UserID, payload.ClientID).
					Update(&OAuthGrant{}, map[string]any{"revoked_at": s.now()})
			})
		}
	}

	_ = s.redis.Del(ctx, key).Err()

	return nil
}

func (s *Server) issue(ctx context.Context, tenantID, userID, clientID, scope, family string) (*TokenOutput, error) {
	access, err := s.issuer.Issue(auth.IssueOptions{
		Subject:  userID,
		Actor:    tenancy.ActorTenant,
		Audience: auth.AudienceMCP,
		TenantID: tenantID,
		ClientID: clientID,
		Scope:    scope,
		TTL:      s.cfg.AccessTTL,
	})
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "issue access token")
	}

	refresh := randomToken(32)
	payload := refreshPayload{TenantID: tenantID, UserID: userID, ClientID: clientID, Scope: scope, Family: family}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "marshal refresh token")
	}
	if err := s.redis.Set(ctx, refreshKeyPrefix+hashToken(refresh), raw, s.cfg.RefreshTTL).Err(); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "store refresh token")
	}

	return &TokenOutput{
		AccessToken:  access,
		TokenType:    "Bearer",
		ExpiresIn:    int(s.cfg.AccessTTL.Seconds()),
		RefreshToken: refresh,
		Scope:        scope,
	}, nil
}

func (s *Server) upsertGrant(ctx context.Context, payload codePayload) error {
	scopeCtx := withScope(ctx, refreshPayload{TenantID: payload.TenantID, UserID: payload.UserID})

	err := s.grants.Tx(scopeCtx, func(tx tenancy.Tx) error {
		var existing OAuthGrant
		findErr := tx.Where("user_id = ? AND client_id = ?", payload.UserID, payload.ClientID).First(&existing)
		switch {
		case findErr == nil:
			return tx.Where("user_id = ? AND client_id = ?", payload.UserID, payload.ClientID).
				Update(&OAuthGrant{}, map[string]any{"revoked_at": nil, "scope": payload.Scope})
		case errors.Is(findErr, gorm.ErrRecordNotFound):
			return tx.Create(&OAuthGrant{UserID: payload.UserID, ClientID: payload.ClientID, Scope: payload.Scope})
		default:
			return findErr
		}
	})
	if err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "upsert grant")
	}

	return nil
}

type clientInfo struct {
	URIs []string
}

func (s *Server) loadClient(ctx context.Context, tenantID, clientID string) (clientInfo, error) {
	var row OAuthClient
	err := s.clients.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Where("client_id = ?", clientID).First(&row)
	})
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && row.TenantID != tenantID) {
		return clientInfo{}, apperr.New(apperr.CodeInvalidArgument, "unknown client_id")
	}
	if err != nil {
		return clientInfo{}, apperr.Wrap(err, apperr.CodeInternal, "load client")
	}
	if row.RevokedAt != nil {
		return clientInfo{}, apperr.New(apperr.CodeInvalidArgument, "client revoked")
	}

	var uris []string
	if err := json.Unmarshal([]byte(row.RedirectURIs), &uris); err != nil {
		return clientInfo{}, apperr.Wrap(err, apperr.CodeInternal, "parse redirect uris")
	}

	return clientInfo{URIs: uris}, nil
}

func withScope(ctx context.Context, payload refreshPayload) context.Context {
	return tenancy.WithScope(ctx, tenancy.Scope{
		TenantID: payload.TenantID,
		UserID:   payload.UserID,
		Actor:    tenancy.ActorTenant,
	})
}

// take 读取并删除，保证授权码 / 刷新令牌只能使用一次。
func (s *Server) take(ctx context.Context, key string) ([]byte, error) {
	raw, err := s.redis.Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}
	_ = s.redis.Del(ctx, key).Err()

	return raw, nil
}

func validateRedirectURI(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return apperr.New(apperr.CodeInvalidArgument, "invalid redirect_uri")
	}
	if parsed.Scheme == "https" {
		return nil
	}
	if parsed.Scheme == "http" && (parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "localhost") {
		return nil
	}
	return apperr.New(apperr.CodeInvalidArgument, "redirect_uri must use https (http allowed for loopback)")
}

func verifyPKCE(verifier, challenge string) bool {
	if verifier == "" || challenge == "" {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	encoded := base64.RawURLEncoding.EncodeToString(sum[:])
	return encoded == challenge
}

func randomToken(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return uuid.Must(uuid.NewV7()).String()
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func scopeOrDefault(scope string) string {
	if strings.TrimSpace(scope) == "" {
		return ScopeMCP
	}
	return scope
}

var _ = fmt.Sprintf
