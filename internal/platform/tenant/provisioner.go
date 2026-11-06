package tenant

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/cuiwenguang/go-saas-lite/internal/security/authz"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// adminSeed 是开通流水线要创建的管理员。
type adminSeed struct {
	UserID       string
	Email        string
	DisplayName  string
	PasswordHash string
}

// provision 在租户连接上完成角色、管理员与绑定的创建。
// 每一步先查后建，重跑不产生脏数据。
func (s *Service) provision(ctx context.Context, tenantID string, admin adminSeed) error {
	scopeCtx := tenancy.WithScope(ctx, tenancy.Scope{
		TenantID: tenantID,
		UserID:   admin.UserID,
		Actor:    tenancy.ActorTenant,
	})

	return s.tenant.Tx(scopeCtx, func(tx tenancy.Tx) error {
		roleIDs := make(map[string]string, len(authz.BuiltinRoles()))
		for code, perms := range authz.BuiltinRoles() {
			roleID, err := ensureRole(tx, code, perms)
			if err != nil {
				return fmt.Errorf("ensure role %s: %w", code, err)
			}
			roleIDs[code] = roleID
		}

		if err := ensureUser(tx, admin); err != nil {
			return fmt.Errorf("ensure admin user: %w", err)
		}

		adminRoleID, ok := roleIDs[authz.RoleAdmin]
		if !ok {
			return fmt.Errorf("admin role was not created")
		}

		if err := bindRole(tx, admin.UserID, adminRoleID); err != nil {
			return fmt.Errorf("bind admin role: %w", err)
		}

		return nil
	})
}

func ensureRole(tx tenancy.Tx, code string, perms []authz.Perm) (string, error) {
	var existing Role
	err := tx.Where("code = ?", code).First(&existing)
	if err == nil {
		return existing.ID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}

	items := make([]string, 0, len(perms))
	for _, perm := range perms {
		items = append(items, perm.String())
	}
	raw, marshalErr := json.Marshal(items)
	if marshalErr != nil {
		return "", marshalErr
	}

	id := uuid.Must(uuid.NewV7()).String()
	row := Role{
		ID:          id,
		Code:        code,
		Name:        code,
		Permissions: string(raw),
		IsBuiltin:   true,
	}
	if createErr := tx.Create(&row); createErr != nil {
		return "", createErr
	}

	return id, nil
}

func ensureUser(tx tenancy.Tx, admin adminSeed) error {
	var existing User
	err := tx.Where("email = ?", admin.Email).First(&existing)
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	return tx.Create(&User{
		ID:                 admin.UserID,
		Email:              admin.Email,
		DisplayName:        admin.DisplayName,
		PasswordHash:       admin.PasswordHash,
		Status:             "active",
		MustChangePassword: true,
	})
}

func bindRole(tx tenancy.Tx, userID, roleID string) error {
	var existing UserRole
	err := tx.Where("user_id = ? AND role_id = ?", userID, roleID).First(&existing)
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	return tx.Create(&UserRole{UserID: userID, RoleID: roleID})
}

// hashPassword 用 bcrypt 存储；初始密码只在创建响应里返回一次。
func hashPassword(plain string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

// generatePassword 生成 16 字节随机密码。
func generatePassword() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
