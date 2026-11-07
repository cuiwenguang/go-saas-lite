package plans

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/cuiwenguang/go-saas-lite/internal/platform/audit"
	"github.com/cuiwenguang/go-saas-lite/internal/quota"
	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

var codePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,31}$`)

// Service 负责套餐定义与配额维护。
// 限额读取走 quota.PlanStore（30s 缓存），因此这里的改动对租户侧最迟 30 秒生效。
type Service struct {
	platform tenancy.PlatformDB
	audit    *audit.Service
}

// NewService 创建套餐服务。
func NewService(platform tenancy.PlatformDB, auditService *audit.Service) *Service {
	return &Service{platform: platform, audit: auditService}
}

// List 返回全部套餐及引用租户数。
func (s *Service) List(ctx context.Context) ([]PlanOutput, error) {
	var rows []quota.Plan
	var counts []planCount

	err := s.platform.Read(ctx, func(tx tenancy.Tx) error {
		if err := tx.Model(&quota.Plan{}).Order("created_at ASC").Find(&rows); err != nil {
			return err
		}
		return tx.Raw(tenantCountsSQL).Scan(&counts)
	})
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "list plans")
	}

	used := make(map[string]int64, len(counts))
	for _, count := range counts {
		used[count.PlanCode] = count.Total
	}

	out := make([]PlanOutput, 0, len(rows))
	for _, plan := range rows {
		out = append(out, toOutput(plan, used[plan.Code]))
	}

	return out, nil
}

// Get 按 ID 返回套餐。
func (s *Service) Get(ctx context.Context, id string) (*PlanOutput, error) {
	var plan quota.Plan
	var count planCount

	err := s.platform.Read(ctx, func(tx tenancy.Tx) error {
		if err := tx.Where("id = ?", id).First(&plan); err != nil {
			return err
		}
		return tx.Raw(tenantCountByCodeSQL, plan.Code).Scan(&count)
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperr.New(apperr.CodePlanNotFound, "plan not found")
	}
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "get plan")
	}

	out := toOutput(plan, count.Total)
	return &out, nil
}

// Create 新建套餐。
func (s *Service) Create(ctx context.Context, in CreateInput) (*PlanOutput, error) {
	code := strings.ToLower(strings.TrimSpace(in.Code))
	if !codePattern.MatchString(code) {
		return nil, apperr.New(apperr.CodeInvalidArgument, "code must match ^[a-z][a-z0-9_-]{1,31}$")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "name is required")
	}
	if err := validateLimits(in.Quota); err != nil {
		return nil, err
	}

	raw, err := json.Marshal(in.Quota)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "encode quota")
	}

	plan := quota.Plan{
		ID:      uuid.Must(uuid.NewV7()).String(),
		Code:    code,
		Name:    name,
		Quota:   string(raw),
		Enabled: true,
	}
	if in.Enabled != nil {
		plan.Enabled = *in.Enabled
	}

	if err := s.platform.Tx(ctx, func(tx tenancy.Tx) error {
		return tx.Create(&plan)
	}); err != nil {
		return nil, conflictOrWrap(err, "create plan")
	}

	s.recordAudit(ctx, audit.Entry{
		Category:   audit.CategoryPlan,
		Action:     audit.ActionPlanCreate,
		ObjectType: "plan",
		ObjectID:   plan.ID,
		Summary:    "新建套餐 " + plan.Name,
		After:      map[string]any{"code": plan.Code, "name": plan.Name, "quota": in.Quota, "enabled": plan.Enabled},
	})

	return s.Get(ctx, plan.ID)
}

// Update 修改套餐名称 / 配额 / 启用状态；code 不可改，避免打断 tenants.plan_code 引用。
func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (*PlanOutput, error) {
	current, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	values := make(map[string]any, 3)
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return nil, apperr.New(apperr.CodeInvalidArgument, "name cannot be empty")
		}
		values["name"] = name
	}
	if in.Quota != nil {
		if err := validateLimits(*in.Quota); err != nil {
			return nil, err
		}
		raw, err := json.Marshal(*in.Quota)
		if err != nil {
			return nil, apperr.Wrap(err, apperr.CodeInternal, "encode quota")
		}
		values["quota"] = string(raw)
	}
	if in.Enabled != nil {
		values["enabled"] = *in.Enabled
	}

	if len(values) == 0 {
		return current, nil
	}

	if err := s.platform.Tx(ctx, func(tx tenancy.Tx) error {
		return tx.Where("id = ?", id).Update(&quota.Plan{}, values)
	}); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "update plan")
	}

	updated, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	action := audit.ActionPlanUpdate
	summary := "修改套餐 " + updated.Name
	if in.Quota == nil && in.Name == nil {
		action = audit.ActionPlanToggle
		summary = "切换套餐启用状态 " + updated.Name
	}
	s.recordAudit(ctx, audit.Entry{
		Category:   audit.CategoryPlan,
		Action:     action,
		ObjectType: "plan",
		ObjectID:   id,
		Summary:    summary,
		Before:     map[string]any{"name": current.Name, "quota": current.Quota, "enabled": current.Enabled},
		After:      map[string]any{"name": updated.Name, "quota": updated.Quota, "enabled": updated.Enabled},
	})

	return updated, nil
}

type planCount struct {
	PlanCode string `gorm:"column:plan_code"`
	Total    int64  `gorm:"column:total"`
}

// recordAudit 写审计日志；失败不能阻断主流程。
func (s *Service) recordAudit(ctx context.Context, entry audit.Entry) {
	if s.audit == nil {
		return
	}
	_ = s.audit.Record(ctx, entry)
}

func toOutput(plan quota.Plan, tenantCount int64) PlanOutput {
	limits, err := quota.ParseLimits(plan.Quota)
	if err != nil {
		limits = quota.Limits{}
	}

	return PlanOutput{
		ID:          plan.ID,
		Code:        plan.Code,
		Name:        plan.Name,
		Quota:       limits,
		Enabled:     plan.Enabled,
		TenantCount: tenantCount,
		CreatedAt:   plan.CreatedAt,
		UpdatedAt:   plan.UpdatedAt,
	}
}

func validateLimits(limits quota.Limits) error {
	if limits.MaxUsers < 0 || limits.StorageGB < 0 || limits.APIQPS < 0 || limits.APICalls < 0 ||
		limits.MCPCalls < 0 || limits.MCPTokens < 0 || limits.MCPSessions < 0 {
		return apperr.New(apperr.CodeInvalidArgument, "quota values cannot be negative")
	}

	return nil
}

func conflictOrWrap(err error, action string) error {
	if strings.Contains(err.Error(), "23505") {
		return apperr.New(apperr.CodeConflict, "plan code already exists")
	}

	return apperr.Wrap(err, apperr.CodeInternal, action)
}
