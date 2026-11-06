package tenants

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/cuiwenguang/go-saas-lite/internal/platform/audit"
	"github.com/cuiwenguang/go-saas-lite/internal/quota"
	"github.com/cuiwenguang/go-saas-lite/internal/security/identity"
	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

var codePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,63}$`)

// Service 负责租户生命周期。
type Service struct {
	platform   tenancy.PlatformDB
	tenant     tenancy.DB
	audit      *audit.Service
	identities *identity.Service
	now        func() time.Time
}

// NewService 创建租户服务。
func NewService(platform tenancy.PlatformDB, tenantDB tenancy.DB, auditService *audit.Service, identities *identity.Service) *Service {
	return &Service{
		platform:   platform,
		tenant:     tenantDB,
		audit:      auditService,
		identities: identities,
		now:        time.Now,
	}
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

	// 全局唯一：邮箱已被别的租户占用时直接拒绝，避免租户建好但管理员登不进来。
	if err := s.ensureEmailAvailable(ctx, in.AdminEmail); err != nil {
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
		ProvisionLog:    "[]",
	}
	tenant.Domain = optional(in.Domain)
	tenant.Subdomain = optional(in.Subdomain)
	tenant.ContactName = optional(in.ContactName)
	tenant.ContactPhone = optional(in.ContactPhone)
	tenant.ContactEmail = optional(in.ContactEmail)
	tenant.AdminEmail = optional(strings.ToLower(strings.TrimSpace(in.AdminEmail)))

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

	if err := s.provision(ctx, tenant.ID, admin, false); err != nil {
		s.markFailed(ctx, tenant.ID, err)
		return nil, "", apperr.Wrap(err, apperr.CodeTenantProvision, "provision tenant failed")
	}
	s.recordStep(ctx, tenant.ID, "tenant_bootstrapped")

	if err := s.indexAdmin(ctx, tenant.ID, admin); err != nil {
		s.markFailed(ctx, tenant.ID, err)
		return nil, "", apperr.Wrap(err, apperr.CodeTenantProvision, "index tenant admin failed")
	}
	s.recordStep(ctx, tenant.ID, "identity_indexed")

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

	s.recordAudit(ctx, audit.Entry{
		Category:   audit.CategoryTenant,
		Action:     audit.ActionTenantCreate,
		ObjectType: "tenant",
		ObjectID:   tenant.ID,
		Summary:    "开通租户 " + tenant.Name,
		After:      map[string]any{"code": tenant.Code, "name": tenant.Name, "plan_code": tenant.PlanCode},
	})

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
		if filter.PlanCode != "" {
			query = query.Where("plan_code = ?", filter.PlanCode)
		}
		if keyword := strings.TrimSpace(strings.ToLower(filter.Keyword)); keyword != "" {
			like := "%" + keyword + "%"
			query = query.Where(
				"LOWER(name) LIKE ? OR LOWER(code) LIKE ? OR LOWER(COALESCE(subdomain, '')) LIKE ?",
				like, like, like,
			)
		}
		if filter.ExpireBefore != nil {
			query = query.Where("expire_at IS NOT NULL AND expire_at <= ?", *filter.ExpireBefore)
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

// Summary 返回各状态租户数量，供列表分段筛选与总览使用。
func (s *Service) Summary(ctx context.Context) (*SummaryOutput, error) {
	type statusCount struct {
		Status string `gorm:"column:status"`
		Total  int64  `gorm:"column:total"`
	}

	var rows []statusCount
	err := s.platform.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Raw(summarySQL).Scan(&rows)
	})
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "tenant summary")
	}

	out := &SummaryOutput{}
	for _, row := range rows {
		out.Total += row.Total
		switch row.Status {
		case StatusPending:
			out.Pending = row.Total
		case StatusActive:
			out.Active = row.Total
		case StatusSuspended:
			out.Suspended = row.Total
		case StatusExpired:
			out.Expired = row.Total
		case StatusArchived:
			out.Archived = row.Total
		}
	}

	return out, nil
}

// Update 修改可编辑字段。续费就是改 expire_at；expired 租户续到未来日期会恢复 active。
func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (*Tenant, error) {
	current, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if current.Status == StatusArchived {
		return nil, apperr.New(apperr.CodeInvalidArgument, "archived tenant cannot be updated")
	}

	values := make(map[string]any, 8)
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return nil, apperr.New(apperr.CodeInvalidArgument, "name cannot be empty")
		}
		values["name"] = name
	}
	if in.PlanCode != nil {
		values["plan_code"] = *in.PlanCode
	}
	if in.Timezone != nil {
		values["timezone"] = *in.Timezone
	}
	if in.Locale != nil {
		values["locale"] = *in.Locale
	}
	if in.ContactName != nil {
		values["contact_name"] = optional(*in.ContactName)
	}
	if in.ContactPhone != nil {
		values["contact_phone"] = optional(*in.ContactPhone)
	}
	if in.ContactEmail != nil {
		values["contact_email"] = optional(*in.ContactEmail)
	}
	if in.ExpireAt != nil {
		values["expire_at"] = *in.ExpireAt
		if current.Status == StatusExpired && in.ExpireAt.After(s.now()) {
			values["status"] = StatusActive
		}
	}
	if len(values) == 0 {
		return current, nil
	}

	if err := s.platform.Tx(ctx, func(tx tenancy.Tx) error {
		return tx.Where("id = ?", id).Update(&Tenant{}, values)
	}); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "update tenant")
	}

	updated, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	action := audit.ActionTenantUpdate
	summary := "修改租户 " + updated.Name
	if in.ExpireAt != nil {
		action = audit.ActionTenantRenew
		summary = "续费租户 " + updated.Name
	}
	s.recordAudit(ctx, audit.Entry{
		Category:   audit.CategoryTenant,
		Action:     action,
		ObjectType: "tenant",
		ObjectID:   id,
		Summary:    summary,
		Before:     map[string]any{"name": current.Name, "plan_code": current.PlanCode, "expire_at": current.ExpireAt, "status": current.Status},
		After:      map[string]any{"name": updated.Name, "plan_code": updated.PlanCode, "expire_at": updated.ExpireAt, "status": updated.Status},
	})

	return updated, nil
}

// RetryProvision 从未成功的断点重跑开通流水线，并重置管理员初始密码（仅返回一次）。
func (s *Service) RetryProvision(ctx context.Context, id string) (*Tenant, string, error) {
	tenant, err := s.Get(ctx, id)
	if err != nil {
		return nil, "", err
	}
	switch {
	case tenant.Status == StatusArchived:
		return nil, "", apperr.New(apperr.CodeInvalidArgument, "archived tenant cannot be provisioned")
	case tenant.ProvisionStatus == ProvisionSucceeded:
		return nil, "", apperr.New(apperr.CodeInvalidArgument, "tenant is already provisioned")
	case tenant.AdminEmail == nil || *tenant.AdminEmail == "":
		return nil, "", apperr.New(apperr.CodeInvalidArgument, "tenant has no recorded admin email")
	}

	password, err := generatePassword()
	if err != nil {
		return nil, "", apperr.Wrap(err, apperr.CodeInternal, "generate password")
	}
	passwordHash, err := hashPassword(password)
	if err != nil {
		return nil, "", apperr.Wrap(err, apperr.CodeInternal, "hash password")
	}

	if err := s.platform.Tx(ctx, func(tx tenancy.Tx) error {
		return tx.Where("id = ?", id).Update(&Tenant{}, map[string]any{"provision_status": ProvisionRunning})
	}); err != nil {
		return nil, "", apperr.Wrap(err, apperr.CodeInternal, "mark provisioning")
	}
	s.recordStep(ctx, id, "provision_retried")

	admin := adminSeed{
		UserID:       uuid.Must(uuid.NewV7()).String(),
		Email:        *tenant.AdminEmail,
		DisplayName:  "管理员",
		PasswordHash: passwordHash,
	}
	if err := s.provision(ctx, id, admin, true); err != nil {
		s.markFailed(ctx, id, err)
		return nil, "", apperr.Wrap(err, apperr.CodeTenantProvision, "provision tenant failed")
	}
	s.recordStep(ctx, id, "tenant_bootstrapped")

	if err := s.indexAdmin(ctx, id, admin); err != nil {
		s.markFailed(ctx, id, err)
		return nil, "", apperr.Wrap(err, apperr.CodeTenantProvision, "index tenant admin failed")
	}
	s.recordStep(ctx, id, "identity_indexed")

	activated := map[string]any{
		"provision_status": ProvisionSucceeded,
		"provision_step":   "done",
	}
	if tenant.Status == StatusPending {
		activated["status"] = StatusActive
	}
	if err := s.platform.Tx(ctx, func(tx tenancy.Tx) error {
		return tx.Where("id = ?", id).Update(&Tenant{}, activated)
	}); err != nil {
		return nil, "", apperr.Wrap(err, apperr.CodeInternal, "activate tenant")
	}

	updated, err := s.Get(ctx, id)
	if err != nil {
		return nil, "", err
	}

	s.recordAudit(ctx, audit.Entry{
		Category:   audit.CategoryTenant,
		Action:     audit.ActionTenantRetry,
		ObjectType: "tenant",
		ObjectID:   id,
		Summary:    "重试开通租户 " + updated.Name,
		After:      map[string]any{"provision_status": updated.ProvisionStatus},
	})

	return updated, password, nil
}

// ResetAdminPassword 重置租户管理员密码；旧令牌因 token_version 递增立即失效。
func (s *Service) ResetAdminPassword(ctx context.Context, id string) (*Tenant, string, error) {
	tenant, err := s.Get(ctx, id)
	if err != nil {
		return nil, "", err
	}
	if tenant.AdminEmail == nil || *tenant.AdminEmail == "" {
		return nil, "", apperr.New(apperr.CodeInvalidArgument, "tenant has no recorded admin email")
	}

	password, err := generatePassword()
	if err != nil {
		return nil, "", apperr.Wrap(err, apperr.CodeInternal, "generate password")
	}
	passwordHash, err := hashPassword(password)
	if err != nil {
		return nil, "", apperr.Wrap(err, apperr.CodeInternal, "hash password")
	}

	scopeCtx := tenancy.WithScope(ctx, tenancy.Scope{TenantID: id, Actor: tenancy.ActorTenant})
	err = s.tenant.Tx(scopeCtx, func(tx tenancy.Tx) error {
		var user User
		if err := tx.Where("email = ?", *tenant.AdminEmail).First(&user); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.New(apperr.CodeTenantNotFound, "tenant admin user not found")
			}
			return err
		}

		return tx.Where("id = ?", user.ID).Update(&User{}, map[string]any{
			"password_hash":        passwordHash,
			"must_change_password": true,
			"token_version":        gorm.Expr("token_version + 1"),
			"status":               "active",
		})
	})
	if err != nil {
		return nil, "", err
	}

	s.recordAudit(ctx, audit.Entry{
		Category:   audit.CategoryTenant,
		Action:     audit.ActionTenantResetPass,
		Level:      audit.LevelWarn,
		ObjectType: "tenant",
		ObjectID:   id,
		Summary:    "重置租户管理员密码 " + tenant.Name,
		After:      map[string]any{"admin_email": deref(tenant.AdminEmail)},
	})

	return tenant, password, nil
}

// Users 返回租户内用户及其角色，供详情页只读展示。
func (s *Service) Users(ctx context.Context, id string) ([]TenantUserOutput, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return nil, err
	}

	scopeCtx := tenancy.WithScope(ctx, tenancy.Scope{TenantID: id, Actor: tenancy.ActorTenant})
	var users []User
	var roles []Role
	var bindings []UserRole

	if err := s.tenant.Read(scopeCtx, func(tx tenancy.Tx) error {
		if err := tx.Model(&User{}).Order("created_at ASC").Find(&users); err != nil {
			return err
		}
		if err := tx.Model(&Role{}).Find(&roles); err != nil {
			return err
		}
		return tx.Model(&UserRole{}).Find(&bindings)
	}); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "list tenant users")
	}

	s.recordAudit(ctx, audit.Entry{
		Category:   audit.CategorySecurity,
		Action:     audit.ActionTenantReadUsers,
		Level:      audit.LevelWarn,
		ObjectType: "tenant",
		ObjectID:   id,
		Summary:    fmt.Sprintf("跨租户查看租户用户（%d 人）", len(users)),
	})

	roleCodes := make(map[string]string, len(roles))
	for _, role := range roles {
		roleCodes[role.ID] = role.Code
	}

	codesByUser := make(map[string][]string, len(users))
	for _, binding := range bindings {
		if code, ok := roleCodes[binding.RoleID]; ok {
			codesByUser[binding.UserID] = append(codesByUser[binding.UserID], code)
		}
	}

	out := make([]TenantUserOutput, 0, len(users))
	for _, user := range users {
		codes := codesByUser[user.ID]
		if codes == nil {
			codes = []string{}
		}
		out = append(out, TenantUserOutput{
			ID:                 user.ID,
			Email:              user.Email,
			DisplayName:        user.DisplayName,
			Status:             user.Status,
			Roles:              codes,
			MustChangePassword: user.MustChangePassword,
			CreatedAt:          user.CreatedAt,
		})
	}

	return out, nil
}

// Usage 返回租户套餐限额、近 30 天用量与近 7 天 MCP 调用趋势。
func (s *Service) Usage(ctx context.Context, id string) (*UsageOutput, error) {
	tenant, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	type metricTotal struct {
		Metric string `gorm:"column:metric"`
		Total  int64  `gorm:"column:total"`
	}

	var totals []metricTotal
	var series []UsagePoint
	var plan quota.Plan

	readErr := s.platform.Read(ctx, func(tx tenancy.Tx) error {
		if err := tx.Raw(tenantUsageSQL, id).Scan(&totals); err != nil {
			return err
		}
		if err := tx.Raw(tenantCallSeriesSQL, id).Scan(&series); err != nil {
			return err
		}
		return tx.Where("code = ?", tenant.PlanCode).First(&plan)
	})
	/* 套餐缺失时限额按 0（不限）处理，不阻断详情页。 */
	if readErr != nil && !errors.Is(readErr, gorm.ErrRecordNotFound) {
		return nil, apperr.Wrap(readErr, apperr.CodeInternal, "load tenant usage")
	}

	limits, parseErr := quota.ParseLimits(plan.Quota)
	if parseErr != nil {
		limits = quota.Limits{}
	}

	var users int64
	scopeCtx := tenancy.WithScope(ctx, tenancy.Scope{TenantID: id, Actor: tenancy.ActorTenant})
	if err := s.tenant.Read(scopeCtx, func(tx tenancy.Tx) error {
		return tx.Model(&User{}).Count(&users)
	}); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "count tenant users")
	}

	out := &UsageOutput{
		PlanCode:    tenant.PlanCode,
		MaxUsers:    limits.MaxUsers,
		MCPCallsCap: limits.MCPCalls,
		MCPTokenCap: limits.MCPTokens,
		APICallQPS:  limits.APIQPS,
		Usage:       UsageCount{Users: users},
		Series:      series,
	}
	for _, item := range totals {
		switch quota.Metric(item.Metric) {
		case quota.MetricMCPCalls:
			out.Usage.MCPCalls = item.Total
		case quota.MetricAPICalls:
			out.Usage.APICalls = item.Total
		case quota.MetricMCPToken:
			out.Usage.MCPTokens = item.Total
		}
	}

	return out, nil
}

// recordAudit 写审计日志；失败不能阻断主流程。
func (s *Service) recordAudit(ctx context.Context, entry audit.Entry) {
	if s.audit == nil {
		return
	}
	_ = s.audit.Record(ctx, entry)
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

	from := current.Status
	current.Status = status

	level := audit.LevelInfo
	if status == StatusArchived {
		level = audit.LevelWarn
	}
	s.recordAudit(ctx, audit.Entry{
		Category:   audit.CategoryTenant,
		Action:     audit.ActionTenantStatus,
		Level:      level,
		ObjectType: "tenant",
		ObjectID:   id,
		Summary:    fmt.Sprintf("变更租户状态 %s → %s", current.Name, status),
		Before:     map[string]any{"status": from},
		After:      map[string]any{"status": status},
	})

	return current, nil
}

// Archive 注销租户：状态置 archived，数据清理由 worker 异步执行（readme 1.4）。
func (s *Service) Archive(ctx context.Context, id string) error {
	if _, err := s.UpdateStatus(ctx, id, StatusArchived); err != nil {
		return err
	}

	// 索引只是登录定位器，清不掉也不会提权（登录时会复查租户状态），失败不阻塞归档。
	if s.identities != nil {
		_ = s.identities.RemoveTenant(ctx, id)
	}

	return nil
}

// ensureEmailAvailable 校验管理员邮箱未被其他租户占用。
func (s *Service) ensureEmailAvailable(ctx context.Context, email string) error {
	if s.identities == nil || email == "" {
		return nil
	}

	_, err := s.identities.Lookup(ctx, identity.TypeEmail, email)
	if err == nil {
		return apperr.New(apperr.CodeConflict, "该邮箱已被其他账号使用")
	}
	if appErr := apperr.From(err); appErr != nil && appErr.Code == apperr.CodeNotFound {
		return nil
	}

	return apperr.Wrap(err, apperr.CodeInternal, "check admin email")
}

// indexAdmin 把管理员邮箱登记进全局索引（失败可经开通重试或对账修复）。
func (s *Service) indexAdmin(ctx context.Context, tenantID string, admin adminSeed) error {
	if s.identities == nil {
		return nil
	}

	return s.identities.Ensure(ctx, tenantID, admin.UserID, []identity.Identifier{
		{Type: identity.TypeEmail, Value: admin.Email},
	})
}

// recordStep 追加一条成功事件并更新当前步骤，便于定位卡在哪一步。
func (s *Service) recordStep(ctx context.Context, tenantID, step string) {
	s.appendProvisionEvent(ctx, tenantID, ProvisionEvent{Step: step, OK: true, At: s.now()}, map[string]any{
		"provision_step": step,
	})
}

func (s *Service) markFailed(ctx context.Context, tenantID string, cause error) {
	message := cause.Error()
	if len(message) > 200 {
		message = message[:200]
	}

	s.appendProvisionEvent(ctx, tenantID, ProvisionEvent{Step: "failed", OK: false, Error: message, At: s.now()}, map[string]any{
		"provision_status": ProvisionFailed,
		"provision_step":   "failed",
	})
}

// appendProvisionEvent 读改写开通时间线；调用方不关心失败（进度记录不阻塞主流程）。
func (s *Service) appendProvisionEvent(ctx context.Context, tenantID string, event ProvisionEvent, extra map[string]any) {
	_ = s.platform.Tx(ctx, func(tx tenancy.Tx) error {
		var current Tenant
		if err := tx.Where("id = ?", tenantID).First(&current); err != nil {
			return err
		}

		events := append(decodeProvisionLog(current.ProvisionLog), event)
		raw, err := json.Marshal(events)
		if err != nil {
			return err
		}

		values := map[string]any{"provision_log": string(raw)}
		for key, value := range extra {
			values[key] = value
		}

		return tx.Where("id = ?", tenantID).Update(&Tenant{}, values)
	})
}

func toOutput(tenant Tenant) TenantOutput {
	return TenantOutput{
		ID:              tenant.ID,
		Code:            tenant.Code,
		Name:            tenant.Name,
		Domain:          deref(tenant.Domain),
		Subdomain:       deref(tenant.Subdomain),
		Status:          tenant.Status,
		PlanCode:        tenant.PlanCode,
		ExpireAt:        tenant.ExpireAt,
		Timezone:        tenant.Timezone,
		Locale:          tenant.Locale,
		ContactName:     deref(tenant.ContactName),
		ContactPhone:    deref(tenant.ContactPhone),
		ContactEmail:    deref(tenant.ContactEmail),
		AdminEmail:      deref(tenant.AdminEmail),
		ProvisionStatus: tenant.ProvisionStatus,
		ProvisionStep:   deref(tenant.ProvisionStep),
		ProvisionLog:    decodeProvisionLog(tenant.ProvisionLog),
		CreatedAt:       tenant.CreatedAt,
		UpdatedAt:       tenant.UpdatedAt,
	}
}

func toOutputs(items []Tenant) []TenantOutput {
	out := make([]TenantOutput, 0, len(items))
	for _, item := range items {
		out = append(out, toOutput(item))
	}

	return out
}

func decodeProvisionLog(raw string) []ProvisionEvent {
	events := make([]ProvisionEvent, 0)
	if strings.TrimSpace(raw) == "" {
		return events
	}
	if err := json.Unmarshal([]byte(raw), &events); err != nil {
		return make([]ProvisionEvent, 0)
	}

	return events
}

func deref(value *string) string {
	if value == nil {
		return ""
	}

	return *value
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
