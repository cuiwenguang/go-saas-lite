// Package tenancy 是租户数据隔离的唯一入口。
//
// 三条规则（readme 3.2）：
//   - Scope 由中间件写入 ctx，下游一律 ScopeFrom(ctx) 读取，不做参数传递；
//   - 业务只能拿到 DB / Tx，拿不到 *gorm.DB；
//   - 租户条件由 GORM Callback 注入，业务代码不写 tenant_id。
package tenancy

import (
	"context"
	"errors"
)

// ActorType 区分平台侧与租户侧身份。
type ActorType string

const (
	ActorPlatform ActorType = "platform"
	ActorTenant   ActorType = "tenant"
)

// Valid 判断 actor 是否为已知类型。
func (a ActorType) Valid() bool {
	return a == ActorPlatform || a == ActorTenant
}

// Scope 是一次请求的租户上下文，唯一来源是中间件写入的 ctx。
type Scope struct {
	TenantID string
	UserID   string    // 租户用户或 Agent 解析出的用户；平台 Actor 可为空
	ClientID string    // Web 可为空；OAuth Agent 为 client_id；API Key 为 apikey:{id}
	Actor    ActorType // platform | tenant
	Role     string    // 平台侧角色（owner / viewer），租户侧为空
	// ActorEmail 仅用于审计日志展示操作人，不参与授权判定。
	ActorEmail string
	// MustChangePassword 为真时，租户链只放行 /me 与改密接口。
	MustChangePassword bool
}

type scopeKey struct{}

// WithScope 把 Scope 写入 ctx。
func WithScope(ctx context.Context, scope Scope) context.Context {
	return context.WithValue(ctx, scopeKey{}, scope)
}

// ScopeFrom 读取 ctx 中的 Scope；缺失即报错（fail closed），不隐式跨租户。
// 平台身份不带租户，允许 TenantID 为空；租户身份必须带租户。
func ScopeFrom(ctx context.Context) (Scope, error) {
	if ctx == nil {
		return Scope{}, ErrNoScope
	}

	scope, ok := ctx.Value(scopeKey{}).(Scope)
	if !ok {
		return Scope{}, ErrNoScope
	}
	if scope.TenantID == "" && scope.Actor != ActorPlatform {
		return Scope{}, ErrEmptyTenantID
	}

	return scope, nil
}

// requireScope 是数据库门面的前置校验：租户库必须有 tenant_id。
func requireScope(ctx context.Context) error {
	scope, err := ScopeFrom(ctx)
	if err != nil {
		return err
	}
	if scope.TenantID == "" {
		return ErrEmptyTenantID
	}

	return nil
}

var (
	// ErrNoScope 表示 ctx 中没有 Scope。
	ErrNoScope = errors.New("tenancy: scope missing from context")
	// ErrEmptyTenantID 表示 Scope 存在但 tenant_id 为空。
	ErrEmptyTenantID = errors.New("tenancy: scope tenant id is empty")
	// ErrNotTenantTable 表示模型没有 tenant_id 列，不能走租户连接。
	ErrNotTenantTable = errors.New("tenancy: model has no tenant_id column")
	// ErrMissingCondition 表示更新 / 删除既没有 Where 也没有主键。
	ErrMissingCondition = errors.New("tenancy: update/delete requires a where condition or primary key")
	// ErrEmptyValues 表示更新字段为空。
	ErrEmptyValues = errors.New("tenancy: update values is empty")
	// ErrRawWrite 表示用 Raw 执行写操作。
	ErrRawWrite = errors.New("tenancy: raw query cannot be used for writes")
	// ErrRawTerminal 表示 Raw 只允许 Scan / Count。
	ErrRawTerminal = errors.New("tenancy: raw query supports only Scan and Count")
)
