package identity

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"gorm.io/gorm"

	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

var emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// Service 维护全局登录索引，并提供登录定位与对账修复。
type Service struct {
	platform tenancy.PlatformDB
	tenantDB tenancy.DB
}

// NewService 创建索引服务。
func NewService(platform tenancy.PlatformDB, tenantDB tenancy.DB) *Service {
	return &Service{platform: platform, tenantDB: tenantDB}
}

// Identifier 是待登记的登录标识。
type Identifier struct {
	Type  string
	Value string
}

// RepairReport 是一次对账的结果。
type RepairReport struct {
	Tenants   int
	Scanned   int
	Added     int
	Removed   int
	Conflicts int
}

// Normalize 规范化登录标识：邮箱小写去空格，手机号去掉分隔符。
func Normalize(kind, raw string) (string, error) {
	value := strings.TrimSpace(raw)
	switch kind {
	case TypeEmail:
		value = strings.ToLower(value)
		if !emailPattern.MatchString(value) {
			return "", apperr.New(apperr.CodeInvalidArgument, "invalid email")
		}
	case TypePhone:
		replacer := strings.NewReplacer(" ", "", "-", "", "(", "", ")", "")
		value = replacer.Replace(value)
		if len(value) < 6 || len(value) > 20 {
			return "", apperr.New(apperr.CodeInvalidArgument, "invalid phone")
		}
	default:
		return "", apperr.New(apperr.CodeInvalidArgument, "unsupported identifier type: "+kind)
	}

	return value, nil
}

// Lookup 用登录标识定位租户与用户；不存在时返回 CodeNotFound。
func (s *Service) Lookup(ctx context.Context, kind, raw string) (*Identity, error) {
	value, err := Normalize(kind, raw)
	if err != nil {
		return nil, err
	}

	var row Identity
	err = s.platform.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Where("identifier_type = ? AND identifier = ?", kind, value).First(&row)
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperr.New(apperr.CodeNotFound, "identity not found")
	}
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "lookup identity")
	}

	return &row, nil
}

// Ensure 幂等登记登录标识。
// 同一标识已指向别的用户 → CodeConflict（由调用方转成业务提示）。
func (s *Service) Ensure(ctx context.Context, tenantID, userID string, identifiers []Identifier) error {
	if tenantID == "" || userID == "" {
		return apperr.New(apperr.CodeInvalidArgument, "tenant and user are required")
	}

	normalized := make([]Identity, 0, len(identifiers))
	for _, item := range identifiers {
		value, err := Normalize(item.Type, item.Value)
		if err != nil {
			return err
		}
		normalized = append(normalized, Identity{
			IdentifierType: item.Type,
			Identifier:     value,
			TenantID:       tenantID,
			UserID:         userID,
		})
	}

	if len(normalized) == 0 {
		return nil
	}

	err := s.platform.Tx(ctx, func(tx tenancy.Tx) error {
		for _, row := range normalized {
			var existing Identity
			err := tx.Where("identifier_type = ? AND identifier = ?", row.IdentifierType, row.Identifier).First(&existing)
			switch {
			case err == nil:
				if existing.TenantID == row.TenantID && existing.UserID == row.UserID {
					continue
				}

				return apperr.New(apperr.CodeConflict, "登录标识已被其他账号使用")
			case errors.Is(err, gorm.ErrRecordNotFound):
				if createErr := tx.Create(&row); createErr != nil {
					if isUniqueViolation(createErr) {
						return apperr.New(apperr.CodeConflict, "登录标识已被其他账号使用")
					}

					return createErr
				}
			default:
				return err
			}
		}

		return nil
	})
	if err != nil {
		if appErr := apperr.From(err); appErr != nil {
			return appErr
		}

		return apperr.Wrap(err, apperr.CodeInternal, "index identity")
	}

	return nil
}

// Remove 删除某个用户的全部索引行（删除成员时调用）。
func (s *Service) Remove(ctx context.Context, tenantID, userID string) error {
	if tenantID == "" || userID == "" {
		return apperr.New(apperr.CodeInvalidArgument, "tenant and user are required")
	}

	if err := s.platform.Tx(ctx, func(tx tenancy.Tx) error {
		return tx.Where("tenant_id = ? AND user_id = ?", tenantID, userID).Delete(&Identity{})
	}); err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "remove identity")
	}

	return nil
}

// RemoveTenant 删除整个租户的索引行（归档 / 注销租户时调用）。
func (s *Service) RemoveTenant(ctx context.Context, tenantID string) error {
	if tenantID == "" {
		return apperr.New(apperr.CodeInvalidArgument, "tenant is required")
	}

	if err := s.platform.Tx(ctx, func(tx tenancy.Tx) error {
		return tx.Where("tenant_id = ?", tenantID).Delete(&Identity{})
	}); err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "remove tenant identities")
	}

	return nil
}

// Repair 对账：补齐缺失的索引、清理指向不存在用户的索引。
// 按租户分批处理，limit 为单次最多处理的租户数（<=0 时默认 200）。
func (s *Service) Repair(ctx context.Context, limit int) (*RepairReport, error) {
	if limit <= 0 {
		limit = 200
	}

	report := &RepairReport{}
	offset := 0
	for {
		var tenantIDs []string
		err := s.platform.Read(ctx, func(tx tenancy.Tx) error {
			return tx.Model(&tenancyTenant{}).Select("id").Order("created_at ASC").Limit(limit).Offset(offset).Pluck("id", &tenantIDs)
		})
		if err != nil {
			return nil, apperr.Wrap(err, apperr.CodeInternal, "list tenants for repair")
		}
		if len(tenantIDs) == 0 {
			break
		}

		for _, tenantID := range tenantIDs {
			if err := s.repairTenant(ctx, tenantID, report); err != nil {
				return nil, err
			}
		}

		report.Tenants += len(tenantIDs)
		offset += limit
	}

	return report, nil
}

// repairTenant 对齐单个租户的 users 与索引。
func (s *Service) repairTenant(ctx context.Context, tenantID string, report *RepairReport) error {
	var users []User
	var rows []Identity

	scopeCtx := tenancy.WithScope(ctx, tenancy.Scope{TenantID: tenantID, Actor: tenancy.ActorTenant})
	if err := s.tenantDB.Read(scopeCtx, func(tx tenancy.Tx) error {
		return tx.Model(&User{}).Find(&users)
	}); err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "list tenant users for repair")
	}

	if err := s.platform.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Where("tenant_id = ?", tenantID).Find(&rows)
	}); err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "list identities for repair")
	}

	userExists := make(map[string]User, len(users))
	for _, user := range users {
		userExists[user.ID] = user
	}

	// 1. 清理孤儿索引（用户已不存在，或邮箱已变）。
	for _, row := range rows {
		user, ok := userExists[row.UserID]
		stale := !ok
		if !stale && row.IdentifierType == TypeEmail && user.Email != row.Identifier {
			stale = true
		}
		if !stale && row.IdentifierType == TypePhone && (user.Phone == nil || *user.Phone != row.Identifier) {
			stale = true
		}
		if !stale {
			continue
		}

		if err := s.platform.Tx(ctx, func(tx tenancy.Tx) error {
			return tx.Where("identifier_type = ? AND identifier = ?", row.IdentifierType, row.Identifier).Delete(&Identity{})
		}); err != nil {
			return apperr.Wrap(err, apperr.CodeInternal, "delete stale identity")
		}
		report.Removed++
	}

	// 2. 补齐缺失的邮箱 / 手机号索引。
	indexed := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		indexed[row.UserID+"|"+row.Identifier] = struct{}{}
	}
	for _, user := range users {
		candidates := make([]Identifier, 0, 2)
		if user.Email != "" {
			candidates = append(candidates, Identifier{Type: TypeEmail, Value: user.Email})
		}
		if user.Phone != nil && *user.Phone != "" {
			candidates = append(candidates, Identifier{Type: TypePhone, Value: *user.Phone})
		}

		for _, candidate := range candidates {
			value, err := Normalize(candidate.Type, candidate.Value)
			if err != nil {
				continue
			}
			if _, ok := indexed[user.ID+"|"+value]; ok {
				continue
			}

			report.Scanned++
			if err := s.Ensure(ctx, tenantID, user.ID, []Identifier{{Type: candidate.Type, Value: value}}); err != nil {
				if appErr := apperr.From(err); appErr != nil && appErr.Code == apperr.CodeConflict {
					report.Conflicts++

					continue
				}

				return err
			}
			report.Added++
		}
	}

	return nil
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "23505")
}
