package testutil

import (
	"testing"

	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// ClearMustChangePassword 让测试账号跳过首次登录强制改密（守卫本身另有专门用例）。
func (h *Harness) ClearMustChangePassword(t *testing.T, tenantID, email string) {
	t.Helper()

	err := h.Tenant.Tx(Context(tenantID), func(tx tenancy.Tx) error {
		return tx.Where("email = ?", email).Update(&User{}, map[string]any{"must_change_password": false})
	})
	if err != nil {
		t.Fatalf("clear must_change_password: %v", err)
	}
}
