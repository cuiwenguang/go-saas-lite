package members

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/cuiwenguang/go-saas-lite/internal/security/authz"
	"github.com/cuiwenguang/go-saas-lite/internal/security/identity"
	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

const (
	statusActive   = "active"
	statusDisabled = "disabled"
	minPasswordLen = 8
)

// Service 负责租户内成员与角色绑定。
type Service struct {
	db         tenancy.DB
	identities *identity.Service
}

// NewService 创建成员服务。
func NewService(db tenancy.DB, identities *identity.Service) *Service {
	return &Service{db: db, identities: identities}
}

// List 分页返回成员及其角色。
func (s *Service) List(ctx context.Context, filter ListFilter) ([]UserOutput, int64, error) {
	page, size := normalizePage(filter.Page, filter.Size)

	var users []User
	var total int64

	err := s.db.Read(ctx, func(tx tenancy.Tx) error {
		query := tx.Model(&User{})
		if filter.Status != "" {
			query = query.Where("status = ?", filter.Status)
		}
		if keyword := strings.ToLower(strings.TrimSpace(filter.Keyword)); keyword != "" {
			like := "%" + keyword + "%"
			query = query.Where("LOWER(email) LIKE ? OR LOWER(display_name) LIKE ?", like, like)
		}
		if err := query.Count(&total); err != nil {
			return err
		}

		return query.Order("created_at ASC").Limit(size).Offset((page - 1) * size).Find(&users)
	})
	if err != nil {
		return nil, 0, apperr.Wrap(err, apperr.CodeInternal, "list members")
	}

	codes, err := s.roleCodesByUser(ctx)
	if err != nil {
		return nil, 0, err
	}

	out := make([]UserOutput, 0, len(users))
	for _, user := range users {
		out = append(out, toOutput(user, codes[user.ID]))
	}

	return out, total, nil
}

// Create 新建成员：密码为空时生成一次性初始密码，并要求首次登录改密。
func (s *Service) Create(ctx context.Context, in CreateInput) (*CredentialOutput, error) {
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if email == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "email is required")
	}
	roleCode := strings.TrimSpace(in.Role)
	if roleCode == "" {
		roleCode = authz.RoleMember
	}
	if roleCode != authz.RoleAdmin && roleCode != authz.RoleMember {
		return nil, apperr.New(apperr.CodeInvalidArgument, "role must be admin or member")
	}

	password := in.Password
	if password == "" {
		generated, err := generatePassword()
		if err != nil {
			return nil, apperr.Wrap(err, apperr.CodeInternal, "generate password")
		}
		password = generated
	}
	if len(password) < minPasswordLen {
		return nil, apperr.New(apperr.CodeInvalidArgument, "password must be at least 8 characters")
	}

	// 邮箱全局唯一：先查索引，避免写索引时才撞唯一键。
	if err := s.ensureEmailAvailable(ctx, email); err != nil {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "hash password")
	}

	displayName := strings.TrimSpace(in.DisplayName)
	if displayName == "" {
		displayName = email
	}

	user := User{
		ID:                 uuid.Must(uuid.NewV7()).String(),
		Email:              email,
		DisplayName:        displayName,
		PasswordHash:       string(hash),
		Status:             statusActive,
		MustChangePassword: true,
	}

	if err := s.db.Tx(ctx, func(tx tenancy.Tx) error {
		var role Role
		if err := tx.Where("code = ?", roleCode).First(&role); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.New(apperr.CodeInvalidArgument, "role not found: "+roleCode)
			}
			return err
		}
		if err := tx.Create(&user); err != nil {
			return err
		}

		return tx.Create(&UserRole{UserID: user.ID, RoleID: role.ID})
	}); err != nil {
		return nil, conflictOrWrap(err, "create member")
	}

	if err := s.indexEmail(ctx, user); err != nil {
		// 索引登记失败时回滚刚建的用户，避免出现“用户存在但登不进来”的脏数据。
		s.deleteUser(ctx, user.ID)

		return nil, err
	}

	out, err := s.get(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	return &CredentialOutput{User: *out, InitialPassword: password}, nil
}

// Update 修改姓名或状态；停用最后一个管理员会被拒绝。
func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (*UserOutput, error) {
	user, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}

	values := make(map[string]any, 2)
	if in.DisplayName != nil {
		name := strings.TrimSpace(*in.DisplayName)
		if name == "" {
			return nil, apperr.New(apperr.CodeInvalidArgument, "display_name cannot be empty")
		}
		values["display_name"] = name
	}
	if in.Status != nil {
		if *in.Status != statusActive && *in.Status != statusDisabled {
			return nil, apperr.New(apperr.CodeInvalidArgument, "status must be active or disabled")
		}
		if *in.Status != statusActive && user.Status == statusActive {
			if err := s.ensureNotLastAdmin(ctx, user.ID); err != nil {
				return nil, err
			}
		}
		values["status"] = *in.Status
	}

	if len(values) == 0 {
		return s.get(ctx, id)
	}

	if err := s.db.Tx(ctx, func(tx tenancy.Tx) error {
		return tx.Where("id = ?", id).Update(&User{}, values)
	}); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "update member")
	}

	return s.get(ctx, id)
}

// ResetPassword 重置成员密码，返回一次性初始密码。
func (s *Service) ResetPassword(ctx context.Context, id string) (string, error) {
	if _, err := s.load(ctx, id); err != nil {
		return "", err
	}

	password, err := generatePassword()
	if err != nil {
		return "", apperr.Wrap(err, apperr.CodeInternal, "generate password")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", apperr.Wrap(err, apperr.CodeInternal, "hash password")
	}

	if err := s.db.Tx(ctx, func(tx tenancy.Tx) error {
		return tx.Where("id = ?", id).Update(&User{}, map[string]any{
			"password_hash":        string(hash),
			"must_change_password": true,
			"token_version":        gorm.Expr("token_version + 1"),
			"refresh_jti":          "",
		})
	}); err != nil {
		return "", apperr.Wrap(err, apperr.CodeInternal, "reset password")
	}

	return password, nil
}

// SetRoles 用新角色集合替换成员的角色绑定。
func (s *Service) SetRoles(ctx context.Context, id string, codes []string) (*UserOutput, error) {
	if _, err := s.load(ctx, id); err != nil {
		return nil, err
	}

	normalized := make([]string, 0, len(codes))
	seen := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code != authz.RoleAdmin && code != authz.RoleMember {
			return nil, apperr.New(apperr.CodeInvalidArgument, "unknown role: "+code)
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		normalized = append(normalized, code)
	}
	if len(normalized) == 0 {
		return nil, apperr.New(apperr.CodeInvalidArgument, "at least one role is required")
	}
	sort.Strings(normalized)

	current, err := s.roleCodesByUser(ctx)
	if err != nil {
		return nil, err
	}
	if containsCode(current[id], authz.RoleAdmin) && !containsCode(normalized, authz.RoleAdmin) {
		if err := s.ensureNotLastAdmin(ctx, id); err != nil {
			return nil, err
		}
	}

	if err := s.db.Tx(ctx, func(tx tenancy.Tx) error {
		roleIDs := make([]string, 0, len(normalized))
		for _, code := range normalized {
			var role Role
			if err := tx.Where("code = ?", code).First(&role); err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return apperr.New(apperr.CodeInvalidArgument, "role not found: "+code)
				}
				return err
			}
			roleIDs = append(roleIDs, role.ID)
		}

		if err := tx.Where("user_id = ?", id).Delete(&UserRole{}); err != nil {
			return err
		}
		for _, roleID := range roleIDs {
			if err := tx.Create(&UserRole{UserID: id, RoleID: roleID}); err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "bind roles")
	}

	return s.get(ctx, id)
}

// Roles 返回内置角色（只读）。
func (s *Service) Roles(ctx context.Context) ([]RoleOutput, error) {
	var roles []Role
	if err := s.db.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Model(&Role{}).Order("code ASC").Find(&roles)
	}); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "list roles")
	}

	out := make([]RoleOutput, 0, len(roles))
	for _, role := range roles {
		out = append(out, RoleOutput{
			ID:          role.ID,
			Code:        role.Code,
			Name:        role.Name,
			Permissions: parsePermissions(role.Permissions),
		})
	}

	return out, nil
}

// ensureEmailAvailable 校验邮箱未被任何租户占用（v1 一人一租户）。
func (s *Service) ensureEmailAvailable(ctx context.Context, email string) error {
	if s.identities == nil {
		return nil
	}

	_, err := s.identities.Lookup(ctx, identity.TypeEmail, email)
	if err == nil {
		return apperr.New(apperr.CodeConflict, "该邮箱已被其他账号使用")
	}
	if appErr := apperr.From(err); appErr != nil && appErr.Code == apperr.CodeNotFound {
		return nil
	}

	return apperr.Wrap(err, apperr.CodeInternal, "check member email")
}

// indexEmail 把成员邮箱登记进全局登录索引。
func (s *Service) indexEmail(ctx context.Context, user User) error {
	if s.identities == nil {
		return nil
	}

	return s.identities.Ensure(ctx, user.TenantID, user.ID, []identity.Identifier{
		{Type: identity.TypeEmail, Value: user.Email},
	})
}

// deleteUser 补偿删除：索引登记失败时把刚建的用户与角色绑定一起清掉。
func (s *Service) deleteUser(ctx context.Context, id string) {
	_ = s.db.Tx(ctx, func(tx tenancy.Tx) error {
		if err := tx.Where("user_id = ?", id).Delete(&UserRole{}); err != nil {
			return err
		}

		return tx.Where("id = ?", id).Delete(&User{})
	})
}

func (s *Service) load(ctx context.Context, id string) (*User, error) {
	var user User
	err := s.db.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Where("id = ?", id).First(&user)
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperr.New(apperr.CodeNotFound, "member not found")
	}
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "load member")
	}

	return &user, nil
}

func (s *Service) get(ctx context.Context, id string) (*UserOutput, error) {
	user, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	codes, err := s.roleCodesByUser(ctx)
	if err != nil {
		return nil, err
	}

	out := toOutput(*user, codes[id])
	return &out, nil
}

// roleCodesByUser 返回 user_id → 角色编码。
func (s *Service) roleCodesByUser(ctx context.Context) (map[string][]string, error) {
	var roles []Role
	var bindings []UserRole

	if err := s.db.Read(ctx, func(tx tenancy.Tx) error {
		if err := tx.Model(&Role{}).Find(&roles); err != nil {
			return err
		}
		return tx.Model(&UserRole{}).Find(&bindings)
	}); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "load role bindings")
	}

	codeByID := make(map[string]string, len(roles))
	for _, role := range roles {
		codeByID[role.ID] = role.Code
	}

	out := make(map[string][]string, len(bindings))
	for _, binding := range bindings {
		if code, ok := codeByID[binding.RoleID]; ok {
			out[binding.UserID] = append(out[binding.UserID], code)
		}
	}
	for id := range out {
		sort.Strings(out[id])
	}

	return out, nil
}

// ensureNotLastAdmin 保证去掉目标用户后，租户仍有至少一个 active 管理员。
func (s *Service) ensureNotLastAdmin(ctx context.Context, excludeUserID string) error {
	var remaining int64

	err := s.db.Read(ctx, func(tx tenancy.Tx) error {
		var adminRole Role
		if err := tx.Where("code = ?", authz.RoleAdmin).First(&adminRole); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}

		var bindings []UserRole
		if err := tx.Model(&UserRole{}).Where("role_id = ?", adminRole.ID).Find(&bindings); err != nil {
			return err
		}
		var users []User
		if err := tx.Model(&User{}).Where("status = ?", statusActive).Find(&users); err != nil {
			return err
		}

		active := make(map[string]struct{}, len(users))
		for _, user := range users {
			active[user.ID] = struct{}{}
		}
		for _, binding := range bindings {
			if binding.UserID == excludeUserID {
				continue
			}
			if _, ok := active[binding.UserID]; ok {
				remaining++
			}
		}

		return nil
	})
	if err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "check admins")
	}
	if remaining == 0 {
		return apperr.New(apperr.CodeInvalidArgument, "at least one active admin is required")
	}

	return nil
}

func toOutput(user User, roles []string) UserOutput {
	if roles == nil {
		roles = []string{}
	}

	return UserOutput{
		ID:                 user.ID,
		Email:              user.Email,
		DisplayName:        user.DisplayName,
		Status:             user.Status,
		Roles:              roles,
		MustChangePassword: user.MustChangePassword,
		CreatedAt:          user.CreatedAt,
	}
}

func parsePermissions(raw string) []string {
	out := make([]string, 0)
	if strings.TrimSpace(raw) == "" {
		return out
	}
	_ = json.Unmarshal([]byte(raw), &out)

	return out
}

func containsCode(codes []string, want string) bool {
	for _, code := range codes {
		if code == want {
			return true
		}
	}

	return false
}

func generatePassword() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func normalizePage(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}

	return page, size
}

func conflictOrWrap(err error, action string) error {
	if strings.Contains(err.Error(), "23505") {
		return apperr.New(apperr.CodeConflict, "member email already exists")
	}

	return apperr.Wrap(err, apperr.CodeInternal, action)
}
