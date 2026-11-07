package audit

import "github.com/cuiwenguang/go-saas-lite/internal/security/authz"

// PermRead 是租户审计查询权限点。
var PermRead = authz.Read("audit")
