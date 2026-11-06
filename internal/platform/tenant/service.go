package tenant

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

var codePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,63}$`)

// Service 负责租户生命周期。
type Service struct {
	platform tenancy.PlatformDB
	tenant   tenancy.DB
	now      func() time.Time
}

// NewService 创建租户服务。
func NewService(platform tenancy.PlatformDB, tenantDB tenancy.DB) *Service {
	return &Service{platform: platform, tenant: tenantDB, now: time.Now}
}

// Create 执行开通流水线，返回租户与仅出现一次的初始密码。
//
// 分两段提交（readme 1.3）：
//  1. 平台库写 tenants(pending)，提交后可被查到；
//  2. 租户库在一个事务内建内置角色、管理员、绑定；
//  3. 平台库置 active / succeeded。
func (s *Service) Create(ctx context.Context, in CreateInput) (*Tenant, string, error) {
	if err := in.validate(); err != nil {
		return nil, "", err
	}

	password := in.AdminPassword
	if password == "" {
		generated, err := generatePassword()
		if err != nil {
			return nil, "", apperr.Wrap(err, apperr.CodeInternal, "generate password")
		}
		password = generated
	}

	passwordHash, err := hashPassword(password)
	if err != nil {
		return nil, "", apperr.Wrap(err, apperr.CodeInternal, "hash password")
	}

	tenant := &Tenant{
		ID:              uuid.Must(uuid.NewV7()).String(),
		Code:            in.Code,
		Name:            in.Name,
		Status:          StatusPending,
		PlanCode:        defaultString(in.PlanCode, "free"),
		Timezone:        in.Timezone,
		Locale:          in.Locale,
		ProvisionStatus: ProvisionRunning,
	}
	tenant.Domain = optional(in.Domain)
	tenant.Subdomain = optional(in.Subdomain)
	tenant.ContactName = optional(in.ContactName)
	tenant.ContactPhone = optional(in.ContactPhone)
	tenant.ContactEmail = optional(in.ContactEmail)

	if err := s.platform.Tx(ctx, func(tx tenancy.Tx) error {
		return tx.Create(tenant)
	}); err != nil {
		return nil, "", conflictOrWrap(err, "create tenant")
	}
	s.recordStep(ctx, tenant.ID, "tenant_created")

	admin := adminSeed{
		UserID:       uuid.Must(uuid.NewV7()).String(),
		Email:        strings.ToLower(in.AdminEmail),
		DisplayName:  defaultString(in.AdminDisplayName, "管理员"),
		PasswordHash: passwordHash,
	}

	if err := s.provision(ctx, tenant.ID, admin); err != nil {
		s.markFailed(ctx, tenant.ID, err)
		return nil, "", apperr.Wrap(err, apperr.CodeTenantProvision, "provision tenant failed")
	}
	s.recordStep(ctx, tenant.ID, "tenant_bootstrapped")

	activated := map[string]any{
		"status":           StatusActive,
		"provision_status": ProvisionSucceeded,
		"provision_step":   "done",
	}
	if err := s.platform.Tx(ctx, func(tx tenancy.Tx) error {
		return tx.Where("id = ?", tenant.ID).Update(&Tenant{}, activated)
	}); err != nil {
		return nil, "", apperr.Wrap(err, apperr.CodeInternal, "activate tenant")
	}

	tenant.Status = StatusActive
	tenant.ProvisionStatus = ProvisionSucceeded

	return tenant, password, nil
}

// List 分页返回租户。
func (s *Service) List(ctx context.Context, filter ListFilter) ([]Tenant, int64, error) {
	page, size := normalizePage(filter.Page, filter.Size)

	var items []Tenant
	var total int64

	err := s.platform.Read(ctx, func(tx tenancy.Tx) error {
		query := tx.Model(&Tenant{})
		if filter.Status != "" {
			query = query.Where("status = ?", filter.Status)
		}

		if err := query.Count(&total); err != nil {
			return err
		}

		return query.Order("created_at DESC").
			Limit(size).
			Offset((page - 1) * size).
			Find(&items)
	})
	if err != nil {
		return nil, 0, apperr.Wrap(err, apperr.CodeInternal, "list tenants")
	}

	return items, total, nil
}

// Get 按 ID 查租户。
func (s *Service) Get(ctx context.Context, id string) (*Tenant, error) {
	var tenant Tenant
	err := s.platform.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Where("id = ?", id).First(&tenant)
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperr.New(apperr.CodeTenantNotFound, "tenant not found")
	}
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "get tenant")
	}

	return &tenant, nil
}

// UpdateStatus 按状态机流转。
func (s *Service) UpdateStatus(ctx context.Context, id, status string) (*Tenant, error) {
	current, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !canTransition(current.Status, status) {
		return nil, apperr.Newf(apperr.CodeInvalidArgument, "cannot change tenant status from %s to %s", current.Status, status).
			WithMeta("from", current.Status).
			WithMeta("to", status)
	}

	if err := s.platform.Tx(ctx, func(tx tenancy.Tx) error {
		return tx.Where("id = ?", id).Update(&Tenant{}, map[string]any{"status": status})
	}); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "update tenant status")
	}

	current.Status = status
	return current, nil
}

// Archive 注销租户：状态置 archived，数据清理由 worker 异步执行（readme 1.4）。
func (s *Service) Archive(ctx context.Context, id string) error {
	if _, err := s.UpdateStatus(ctx, id, StatusArchived); err != nil {
		return err
	}
	return nil
}

// recordStep 记录开通进度，便于定位卡在哪一步。
func (s *Service) recordStep(ctx context.Context, tenantID, step string) {
	_ = s.platform.Tx(ctx, func(tx tenancy.Tx) error {
		return tx.Where("id = ?", tenantID).Update(&Tenant{}, map[string]any{"provision_step": step})
	})
}

func (s *Service) markFailed(ctx context.Context, tenantID string, cause error) {
	step := cause.Error()
	if len(step) > 60 {
		step = step[:60]
	}

	_ = s.platform.Tx(ctx, func(tx tenancy.Tx) error {
		return tx.Where("id = ?", tenantID).Update(&Tenant{}, map[string]any{
			"provision_status": ProvisionFailed,
			"provision_step":   step,
		})
	})
}

func (in CreateInput) validate() error {
	if !codePattern.MatchString(in.Code) {
		return apperr.New(apperr.CodeInvalidArgument, "code must match ^[a-z0-9][a-z0-9-]{1,63}$")
	}
	if strings.TrimSpace(in.Name) == "" {
		return apperr.New(apperr.CodeInvalidArgument, "name is required")
	}
	if in.Timezone == "" {
		return apperr.New(apperr.CodeInvalidArgument, "timezone is required")
	}
	if in.Locale == "" {
		return apperr.New(apperr.CodeInvalidArgument, "locale is required")
	}
	if in.AdminEmail == "" {
		return apperr.New(apperr.CodeInvalidArgument, "admin_email is required")
	}
	if in.AdminPassword != "" && len(in.AdminPassword) < 8 {
		return apperr.New(apperr.CodeInvalidArgument, "admin_password must be at least 8 characters")
	}

	return nil
}

func conflictOrWrap(err error, action string) error {
	if strings.Contains(err.Error(), "23505") {
		return apperr.New(apperr.CodeConflict, "tenant code or domain already exists")
	}
	return apperr.Wrap(err, apperr.CodeInternal, action)
}

func optional(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func normalizePage(page, size int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	return page, size
}
