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

	"github.com/cuiwenguang/go-saas-lite/internal/quota"
	"github.com/cuiwenguang/go-saas-lite/internal/security/authz"
	"github.com/cuiwenguang/go-saas-lite/internal/security/identity"
	auditlog "github.com/cuiwenguang/go-saas-lite/internal/tenant/audit"
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
	quota      *quota.Service
	audit      *auditlog.Service
}

// NewService 创建成员服务；quota / audit 为 nil 时对应能力关闭。
func NewService(db tenancy.DB, identities *identity.Service, quotaService *quota.Service, auditService *auditlog.Service) *Service {
	return &Service{db: db, identities: identities, quota: quotaService, audit: auditService}
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
			query = query.Where("LOWER(email) LIKE ? OR LOWER(COALESCE(phone, '')) LIKE ? OR LOWER(display_name) LIKE ?", like, like, like)
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
	phone, err := normalizeMemberPhone(in.Phone)
	if err != nil {
		return nil, err
	}
	if email == "" && phone == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "email or phone is required")
	}
	roleCode := strings.TrimSpace(in.Role)
	if roleCode == "" {
		roleCode = authz.RoleMember
	}
	if roleCode != authz.RoleAdmin && roleCode != authz.RoleMember {
		return nil, apperr.New(apperr.CodeInvalidArgument, "role must be admin or member")
	}

	if err := s.checkUserQuota(ctx); err != nil {
		return nil, err
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

	// 登录标识全局唯一：先查索引，避免写索引时才撞唯一键。
	if err := s.ensureIdentifierAvailable(ctx, email, phone); err != nil {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "hash password")
	}

	displayName := strings.TrimSpace(in.DisplayName)
	if displayName == "" {
		displayName = email
		if displayName == "" {
			displayName = phone
		}
	}

	user := User{
		ID:                 uuid.Must(uuid.NewV7()).String(),
		Email:              email,
		Phone:              optionalString(phone),
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

	if err := s.indexIdentifiers(ctx, user); err != nil {
		// 索引登记失败时回滚刚建的用户，避免出现“用户存在但登不进来”的脏数据。
		s.deleteUser(ctx, user.ID)

		return nil, err
	}

	out, err := s.get(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	s.recordAudit(ctx, auditlog.Entry{
		Category:   auditlog.CategorySecurity,
		Action:     auditlog.ActionUserCreate,
		ObjectType: "user",
		ObjectID:   user.ID,
		Summary:    "创建成员 " + displayName,
		After:      map[string]any{"email": email, "phone": phone, "role": roleCode},
	})

	return &CredentialOutput{User: *out, InitialPassword: password}, nil
}

// checkUserQuota 校验租户用户数是否已达套餐上限；上限为 0 或未配置配额服务时跳过。
func (s *Service) checkUserQuota(ctx context.Context) error {
	if s.quota == nil {
		return nil
	}

	scope, err := tenancy.ScopeFrom(ctx)
	if err != nil {
		return err
	}

	limits, err := s.quota.Limits(ctx, scope.TenantID)
	if err != nil {
		return err
	}
	if limits.MaxUsers <= 0 {
		return nil
	}

	var count int64
	if err := s.db.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Model(&User{}).Count(&count)
	}); err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "count tenant users")
	}

	if count >= int64(limits.MaxUsers) {
		return apperr.Newf(apperr.CodeQuotaExceeded, "user limit reached").
			WithMeta("metric", "max_users").
			WithMeta("limit", limits.MaxUsers).
			WithMeta("used", count)
	}

	return nil
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

	updated, err := s.get(ctx, id)
	if err != nil {
		return nil, err
	}
	s.recordAudit(ctx, auditlog.Entry{
		Category:   auditlog.CategorySecurity,
		Action:     auditlog.ActionUserUpdate,
		ObjectType: "user",
		ObjectID:   id,
		Summary:    "修改成员 " + updated.DisplayName,
		Before:     map[string]any{"display_name": user.DisplayName, "status": user.Status},
		After:      map[string]any{"display_name": updated.DisplayName, "status": updated.Status},
	})

	return updated, nil
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

	s.recordAudit(ctx, auditlog.Entry{
		Category:   auditlog.CategorySecurity,
		Action:     auditlog.ActionUserResetPassword,
		Level:      auditlog.LevelWarn,
		ObjectType: "user",
		ObjectID:   id,
		Summary:    "重置成员密码",
	})

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

	updated, err := s.get(ctx, id)
	if err != nil {
		return nil, err
	}
	s.recordAudit(ctx, auditlog.Entry{
		Category:   auditlog.CategorySecurity,
		Action:     auditlog.ActionUserSetRoles,
		ObjectType: "user",
		ObjectID:   id,
		Summary:    "变更成员角色 " + updated.DisplayName,
		After:      map[string]any{"roles": updated.Roles},
	})

	return updated, nil
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

// ensureIdentifierAvailable 校验邮箱 / 手机号未被任何租户占用（v1 一人一租户）。
func (s *Service) ensureIdentifierAvailable(ctx context.Context, email, phone string) error {
	if s.identities == nil {
		return nil
	}

	checks := []struct {
		kind  string
		value string
		label string
	}{
		{identity.TypeEmail, email, "邮箱"},
		{identity.TypePhone, phone, "手机号"},
	}

	for _, item := range checks {
		if item.value == "" {
			continue
		}

		_, err := s.identities.Lookup(ctx, item.kind, item.value)
		if err == nil {
			return apperr.New(apperr.CodeConflict, "该"+item.label+"已被其他账号使用")
		}
		if appErr := apperr.From(err); appErr != nil && appErr.Code == apperr.CodeNotFound {
			continue
		}

		return apperr.Wrap(err, apperr.CodeInternal, "check member identifier")
	}

	return nil
}

// indexIdentifiers 把成员邮箱 / 手机号登记进全局登录索引。
func (s *Service) indexIdentifiers(ctx context.Context, user User) error {
	if s.identities == nil {
		return nil
	}

	identifiers := make([]identity.Identifier, 0, 2)
	if user.Email != "" {
		identifiers = append(identifiers, identity.Identifier{Type: identity.TypeEmail, Value: user.Email})
	}
	if user.Phone != nil && *user.Phone != "" {
		identifiers = append(identifiers, identity.Identifier{Type: identity.TypePhone, Value: *user.Phone})
	}
	if len(identifiers) == 0 {
		return nil
	}

	return s.identities.Ensure(ctx, user.TenantID, user.ID, identifiers)
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
		Phone:              derefString(user.Phone),
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

func (s *Service) recordAudit(ctx context.Context, entry auditlog.Entry) {
	if s.audit == nil {
		return
	}
	_ = s.audit.Record(ctx, entry)
}

func normalizeMemberPhone(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}

	return identity.Normalize(identity.TypePhone, raw)
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
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
