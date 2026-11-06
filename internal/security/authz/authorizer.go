package authz

import (
	"context"
	"sync"
	"time"

	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// Authorizer 是业务唯一依赖的授权接口。
type Authorizer interface {
	// Enforce 判断当前用户是否拥有 obj:act，无权限返回 2xxxx 错误。
	Enforce(ctx context.Context, obj, act string) error
	// HasRole 判断当前用户是否拥有指定角色。
	HasRole(ctx context.Context, code string) bool
}

// Option 调整求值服务。
type Option func(*Service)

// WithTTL 设置进程内缓存有效期（默认 1 分钟）。
func WithTTL(ttl time.Duration) Option {
	return func(s *Service) {
		if ttl > 0 {
			s.ttl = ttl
		}
	}
}

// WithClock 便于测试控制时间。
func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

type tenantState struct {
	roles     map[string]map[Perm]struct{}
	userRoles map[string][]string
	expiresAt time.Time
}

// Service 实现 Authorizer，按 tenant 缓存角色与绑定。
// 权限变更后必须调用 Invalidate（多副本通过 redis pub/sub 广播）。
type Service struct {
	src         Source
	broadcaster Broadcaster
	ttl         time.Duration
	now         func() time.Time

	mu      sync.RWMutex
	tenants map[string]*tenantState
}

var _ Authorizer = (*Service)(nil)

// NewService 创建求值服务。
func NewService(src Source, opts ...Option) *Service {
	service := &Service{
		src:     src,
		ttl:     time.Minute,
		now:     time.Now,
		tenants: make(map[string]*tenantState),
	}
	for _, opt := range opts {
		opt(service)
	}

	return service
}

// Enforce 按 ctx 里的 Scope 求值。
func (s *Service) Enforce(ctx context.Context, obj, act string) error {
	scope, err := tenancy.ScopeFrom(ctx)
	if err != nil {
		return apperr.New(apperr.CodeUnauthenticated, "missing tenant scope")
	}
	if scope.UserID == "" {
		return apperr.New(apperr.CodeUnauthenticated, "missing user in scope")
	}

	perm := Perm{Obj: obj, Act: act}
	if err := perm.Validate(); err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "invalid permission")
	}

	state, err := s.state(ctx, scope.TenantID, scope.UserID)
	if err != nil {
		return err
	}

	for _, code := range state.userRoles[scope.UserID] {
		if allows(state.roles[code], perm) {
			return nil
		}
	}

	return apperr.New(apperr.CodePermissionDenied, "permission denied").
		WithMeta("required", perm.String())
}

// HasRole 判断当前用户是否拥有指定角色。
func (s *Service) HasRole(ctx context.Context, code string) bool {
	scope, err := tenancy.ScopeFrom(ctx)
	if err != nil || scope.UserID == "" {
		return false
	}

	state, err := s.state(ctx, scope.TenantID, scope.UserID)
	if err != nil {
		return false
	}

	for _, role := range state.userRoles[scope.UserID] {
		if role == code {
			return true
		}
	}

	return false
}

// Invalidate 丢弃某租户的缓存；绑定或权限变更后必须调用。
func (s *Service) Invalidate(tenantID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tenants, tenantID)
}

// InvalidateAll 丢弃全部缓存。
func (s *Service) InvalidateAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tenants = make(map[string]*tenantState)
}

// state 返回当前租户 + 当前用户的快照；roles 部分构建后不再修改，可安全共享。
func (s *Service) state(ctx context.Context, tenantID, userID string) (*tenantState, error) {
	if snapshot := s.cached(tenantID, userID); snapshot != nil {
		return snapshot, nil
	}

	roles, err := s.src.TenantRoles(ctx)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "load roles")
	}

	rolePerms := make(map[string]map[Perm]struct{}, len(roles))
	for _, role := range roles {
		set := make(map[Perm]struct{}, len(role.Perms))
		for _, perm := range role.Perms {
			set[perm] = struct{}{}
		}
		rolePerms[role.Code] = set
	}

	codes, err := s.src.UserRoleCodes(ctx, userID)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "load user roles")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 并发下可能已被其他请求刷新：复用角色集，只补当前用户绑定。
	if existing, ok := s.tenants[tenantID]; ok && s.now().Before(existing.expiresAt) {
		existing.userRoles[userID] = codes
		return &tenantState{
			roles:     existing.roles,
			userRoles: map[string][]string{userID: codes},
			expiresAt: existing.expiresAt,
		}, nil
	}

	s.tenants[tenantID] = &tenantState{
		roles:     rolePerms,
		userRoles: map[string][]string{userID: codes},
		expiresAt: s.now().Add(s.ttl),
	}

	return &tenantState{
		roles:     rolePerms,
		userRoles: map[string][]string{userID: codes},
		expiresAt: s.now().Add(s.ttl),
	}, nil
}

// cached 命中条件是：租户未过期，且当前用户绑定已加载。
func (s *Service) cached(tenantID, userID string) *tenantState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state, ok := s.tenants[tenantID]
	if !ok || s.now().After(state.expiresAt) {
		return nil
	}

	codes, ok := state.userRoles[userID]
	if !ok {
		return nil
	}

	return &tenantState{
		roles:     state.roles,
		userRoles: map[string][]string{userID: codes},
		expiresAt: state.expiresAt,
	}
}

// allows 判断角色权限集中是否命中，`*:*` 代表全部权限。
func allows(set map[Perm]struct{}, perm Perm) bool {
	if set == nil {
		return false
	}
	if _, ok := set[Wildcard()]; ok {
		return true
	}
	_, ok := set[perm]

	return ok
}
