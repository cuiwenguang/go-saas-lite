package testutil

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// Plan 映射平台组 plans 表。
type Plan struct {
	ID        string `gorm:"column:id;primaryKey"`
	Code      string `gorm:"column:code"`
	Name      string `gorm:"column:name"`
	Quota     string `gorm:"column:quota"`
	Enabled   bool   `gorm:"column:enabled"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableName 固定表名。
func (Plan) TableName() string { return "plans" }

// SetPlan 为租户绑定一个带指定配额 JSON 的套餐。
func (h *Harness) SetPlan(t *testing.T, tenantID, quotaJSON string) error {
	t.Helper()

	hex := strings.ReplaceAll(uuid.Must(uuid.NewV7()).String(), "-", "")
	code := "plan-" + hex[len(hex)-10:]

	if err := h.Platform.Tx(context.Background(), func(tx tenancy.Tx) error {
		return tx.Create(&Plan{ID: uuid.Must(uuid.NewV7()).String(), Code: code, Name: code, Quota: quotaJSON, Enabled: true})
	}); err != nil {
		t.Fatalf("create plan: %v", err)
	}

	return h.Platform.Tx(context.Background(), func(tx tenancy.Tx) error {
		return tx.Where("id = ?", tenantID).Update(&Tenant{}, map[string]any{"plan_code": code})
	})
}

// DailyUsage 读取落库后的每日用量。
func (h *Harness) DailyUsage(t *testing.T, tenantID, metric string) int64 {
	t.Helper()

	var value int64
	if err := h.PlatformGorm.WithContext(context.Background()).
		Raw("SELECT value FROM usage_daily WHERE tenant_id = ? AND metric = ?", tenantID, metric).
		Scan(&value).Error; err != nil {
		t.Fatalf("read usage_daily: %v", err)
	}

	return value
}
