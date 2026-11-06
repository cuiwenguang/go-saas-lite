// Package plans 提供平台侧套餐管理（readme 5：套餐 = 一组配额上限）。
package plans

import (
	"time"

	"github.com/cuiwenguang/go-saas-lite/internal/quota"
)

// CreateInput 是新建套餐入参；code 创建后不可改。
type CreateInput struct {
	Code    string       `json:"code"`
	Name    string       `json:"name"`
	Quota   quota.Limits `json:"quota"`
	Enabled *bool        `json:"enabled"`
}

// UpdateInput 是修改套餐入参；字段为 nil 表示不改。
type UpdateInput struct {
	Name    *string       `json:"name"`
	Quota   *quota.Limits `json:"quota"`
	Enabled *bool         `json:"enabled"`
}

// PlanOutput 是套餐出参；tenant_count 为引用该套餐的租户数。
type PlanOutput struct {
	ID          string       `json:"id"`
	Code        string       `json:"code"`
	Name        string       `json:"name"`
	Quota       quota.Limits `json:"quota"`
	Enabled     bool         `json:"enabled"`
	TenantCount int64        `json:"tenant_count"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}
