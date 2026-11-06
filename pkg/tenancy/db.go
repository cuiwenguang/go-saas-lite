package tenancy

import (
	"context"

	"gorm.io/gorm"
)

// DB 是租户侧唯一的数据访问门面：开事务、校验 Scope、把 tenant_id 交给 Callback。
type DB interface {
	// Tx 在事务中执行 fn；Scope 从 ctx 取，缺失直接报错。
	Tx(ctx context.Context, fn func(tx Tx) error) error
	// Read 是只读路径，同样要求 ctx 里有 Scope。
	Read(ctx context.Context, fn func(tx Tx) error) error
}

type gormDB struct {
	gorm *gorm.DB
}

// NewDB 用已注册租户 Callback 的连接构造租户门面。
func NewDB(gdb *gorm.DB) DB {
	return &gormDB{gorm: gdb}
}

func (d *gormDB) Tx(ctx context.Context, fn func(tx Tx) error) error {
	if err := requireScope(ctx); err != nil {
		return err
	}

	return d.gorm.WithContext(ctx).Transaction(func(g *gorm.DB) error {
		return fn(&tx{gorm: g})
	})
}

func (d *gormDB) Read(ctx context.Context, fn func(tx Tx) error) error {
	if err := requireScope(ctx); err != nil {
		return err
	}

	return fn(&tx{gorm: d.gorm.WithContext(ctx).Session(&gorm.Session{})})
}

// PlatformDB 是平台侧门面：独立连接池、不注册租户 Callback、不注入 tenant_id。
type PlatformDB interface {
	Tx(ctx context.Context, fn func(tx Tx) error) error
	Read(ctx context.Context, fn func(tx Tx) error) error
}

type platformDB struct {
	gorm *gorm.DB
}

// NewPlatformDB 用未注册租户 Callback 的连接构造平台门面。
func NewPlatformDB(gdb *gorm.DB) PlatformDB {
	return &platformDB{gorm: gdb}
}

func (d *platformDB) Tx(ctx context.Context, fn func(tx Tx) error) error {
	return d.gorm.WithContext(ctx).Transaction(func(g *gorm.DB) error {
		return fn(&tx{gorm: g})
	})
}

func (d *platformDB) Read(ctx context.Context, fn func(tx Tx) error) error {
	return fn(&tx{gorm: d.gorm.WithContext(ctx).Session(&gorm.Session{})})
}
