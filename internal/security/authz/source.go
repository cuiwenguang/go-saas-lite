package authz

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// Role 是求值用的角色快照。
type Role struct {
	Code  string
	Perms []Perm
}

// Source 提供角色定义与用户绑定，实现可换（DB、缓存、测试桩）。
type Source interface {
	// TenantRoles 返回当前租户的全部角色；租户由 ctx 里的 Scope 决定。
	TenantRoles(ctx context.Context) ([]Role, error)
	// UserRoleCodes 返回用户在租户内绑定的角色 code。
	UserRoleCodes(ctx context.Context, userID string) ([]string, error)
}

// roleRecord 映射 roles 表（本包私有投影）。
type roleRecord struct {
	ID          string    `gorm:"column:id;primaryKey"`
	TenantID    string    `gorm:"column:tenant_id"`
	Code        string    `gorm:"column:code"`
	Name        string    `gorm:"column:name"`
	Permissions string    `gorm:"column:permissions"`
	IsBuiltin   bool      `gorm:"column:is_builtin"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func (roleRecord) TableName() string { return "roles" }

// userRoleRecord 映射 user_roles 表（本包私有投影）。
type userRoleRecord struct {
	TenantID  string    `gorm:"column:tenant_id"`
	UserID    string    `gorm:"column:user_id"`
	RoleID    string    `gorm:"column:role_id"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (userRoleRecord) TableName() string { return "user_roles" }

type gormSource struct {
	db tenancy.DB
}

// NewGormSource 用租户门面读取角色数据。
func NewGormSource(db tenancy.DB) Source {
	return &gormSource{db: db}
}

func (s *gormSource) TenantRoles(ctx context.Context) ([]Role, error) {
	var rows []roleRecord
	err := s.db.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Model(&roleRecord{}).Find(&rows)
	})
	if err != nil {
		return nil, fmt.Errorf("load roles: %w", err)
	}

	roles := make([]Role, 0, len(rows))
	for _, row := range rows {
		perms, parseErr := parsePermissions(row.Permissions)
		if parseErr != nil {
			return nil, fmt.Errorf("role %s: %w", row.Code, parseErr)
		}
		roles = append(roles, Role{Code: row.Code, Perms: perms})
	}

	return roles, nil
}

func (s *gormSource) UserRoleCodes(ctx context.Context, userID string) ([]string, error) {
	var roleIDs []string
	err := s.db.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Model(&userRoleRecord{}).
			Where("user_id = ?", userID).
			Pluck("role_id", &roleIDs)
	})
	if err != nil {
		return nil, fmt.Errorf("load user roles: %w", err)
	}
	if len(roleIDs) == 0 {
		return nil, nil
	}

	var rows []roleRecord
	err = s.db.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Model(&roleRecord{}).Where("id IN ?", roleIDs).Find(&rows)
	})
	if err != nil {
		return nil, fmt.Errorf("load role codes: %w", err)
	}

	codes := make([]string, 0, len(rows))
	for _, row := range rows {
		codes = append(codes, row.Code)
	}

	return codes, nil
}

func parsePermissions(raw string) ([]Perm, error) {
	if raw == "" {
		return nil, nil
	}

	var items []string
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, fmt.Errorf("parse permissions json: %w", err)
	}

	perms := make([]Perm, 0, len(items))
	for _, item := range items {
		perm, err := Parse(item)
		if err != nil {
			return nil, err
		}
		perms = append(perms, perm)
	}

	return perms, nil
}
