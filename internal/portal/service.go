// Package portal 提供产品介绍、价格与自助注册页面。
package portal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"

	"github.com/cuiwenguang/go-saas-lite/internal/platform/tenants"
	"github.com/cuiwenguang/go-saas-lite/internal/quota"
	"github.com/cuiwenguang/go-saas-lite/internal/security/identity"
	"github.com/cuiwenguang/go-saas-lite/internal/security/otp"
	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

var subdomainPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}$`)

// Options 是门户装配参数。
type Options struct {
	ConsoleURL        string
	AutoApprove       bool
	SignupRatePerHour int
}

// Service 负责价格读取与自助注册。
type Service struct {
	tenants  *tenants.Service
	otp      *otp.Service
	platform tenancy.PlatformDB
	opts     Options
}

// NewService 创建门户服务。
func NewService(tenantsService *tenants.Service, otpService *otp.Service, platform tenancy.PlatformDB, opts Options) *Service {
	return &Service{tenants: tenantsService, otp: otpService, platform: platform, opts: opts}
}

// PlanView 是价格页展示的套餐。
type PlanView struct {
	Code   string
	Name   string
	Limits quota.Limits
}

type planRow struct {
	Code  string `gorm:"column:code"`
	Name  string `gorm:"column:name"`
	Quota string `gorm:"column:quota"`
}

func (planRow) TableName() string { return "plans" }

// Plans 返回已启用的套餐。
func (s *Service) Plans(ctx context.Context) ([]PlanView, error) {
	var rows []planRow
	err := s.platform.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Model(&planRow{}).Where("enabled = ?", true).Order("code ASC").Find(&rows)
	})
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "list portal plans")
	}

	out := make([]PlanView, 0, len(rows))
	for _, row := range rows {
		limits, parseErr := quota.ParseLimits(row.Quota)
		if parseErr != nil {
			limits = quota.Limits{}
		}
		out = append(out, PlanView{Code: row.Code, Name: row.Name, Limits: limits})
	}

	return out, nil
}

// SignupInput 是自助注册入参。
type SignupInput struct {
	CompanyName string
	Phone       string
	Code        string
	Password    string
	Subdomain   string
	AcceptTerms bool
}

// SignupResult 是注册结果。
type SignupResult struct {
	TenantID string
	Code     string
	Name     string
	Pending  bool
}

// SendSignupCode 发送注册验证码。
func (s *Service) SendSignupCode(ctx context.Context, phone string) error {
	if s.otp == nil {
		return apperr.New(apperr.CodeUnavailable, "验证码服务未启用")
	}

	if err := s.otp.Send(ctx, otp.SceneSignup, phone); err != nil {
		if errors.Is(err, otp.ErrTooFrequent) || errors.Is(err, otp.ErrDailyLimit) {
			return apperr.New(apperr.CodeQuotaExceeded, "验证码发送过于频繁")
		}
		return apperr.Wrap(err, apperr.CodeInvalidArgument, "send signup code")
	}

	return nil
}

// Signup 校验验证码并开通租户。
func (s *Service) Signup(ctx context.Context, in SignupInput) (*SignupResult, error) {
	company := strings.TrimSpace(in.CompanyName)
	if len(company) < 2 || len(company) > 64 {
		return nil, apperr.New(apperr.CodeInvalidArgument, "公司名称长度需在 2-64 个字符之间")
	}
	if len(in.Password) < 8 {
		return nil, apperr.New(apperr.CodeInvalidArgument, "密码至少 8 位")
	}
	if !in.AcceptTerms {
		return nil, apperr.New(apperr.CodeInvalidArgument, "请先同意服务条款")
	}

	phone, err := identity.Normalize(identity.TypePhone, in.Phone)
	if err != nil {
		return nil, apperr.New(apperr.CodeInvalidArgument, "手机号格式不正确")
	}

	subdomain := strings.ToLower(strings.TrimSpace(in.Subdomain))
	if subdomain != "" && !subdomainPattern.MatchString(subdomain) {
		return nil, apperr.New(apperr.CodeInvalidArgument, "子域名只能包含小写字母、数字和中划线")
	}

	if s.otp == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "验证码服务未启用")
	}
	if err := s.otp.Verify(ctx, otp.SceneSignup, phone, in.Code); err != nil {
		return nil, apperr.New(apperr.CodeInvalidArgument, "验证码错误或已过期")
	}

	base := tenants.CreateInput{
		Name:             company,
		Timezone:         "Asia/Shanghai",
		Locale:           "zh-CN",
		Subdomain:        subdomain,
		PlanCode:         "free",
		AdminPhone:       phone,
		AdminDisplayName: company + "管理员",
		AdminPassword:    in.Password,
	}

	var lastErr error
	for i := 0; i < 3; i++ {
		base.Code = tenantCode(company)

		var (
			tenant    *tenants.Tenant
			createErr error
		)
		if s.opts.AutoApprove {
			tenant, _, createErr = s.tenants.Create(ctx, base)
		} else {
			tenant, _, createErr = s.tenants.CreatePending(ctx, base)
		}

		if createErr != nil {
			if apperr.Is(createErr, apperr.CodeConflict) {
				lastErr = createErr
				continue
			}
			return nil, createErr
		}

		return &SignupResult{
			TenantID: tenant.ID,
			Code:     tenant.Code,
			Name:     tenant.Name,
			Pending:  !s.opts.AutoApprove,
		}, nil
	}

	return nil, apperr.Wrap(lastErr, apperr.CodeConflict, "租户编码生成冲突，请重试")
}

// tenantCode 从公司名生成租户编码；中文名回退为 t- 前缀。
func tenantCode(name string) string {
	slug := slugify(name)
	if slug == "" {
		slug = "t"
	}

	suffix := randomHex(6)
	max := 64 - len(suffix) - 1
	if len(slug) > max {
		slug = slug[:max]
	}

	return slug + "-" + suffix
}

func slugify(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var builder strings.Builder
	lastDash := false
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			builder.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && builder.Len() > 0 {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}

	return strings.Trim(builder.String(), "-")
}

func randomHex(size int) string {
	buf := make([]byte, (size+1)/2)
	if _, err := rand.Read(buf); err != nil {
		return "000000"
	}

	return hex.EncodeToString(buf)[:size]
}
