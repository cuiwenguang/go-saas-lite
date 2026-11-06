// Package migrations 以嵌入方式提供平台组与租户组的迁移脚本。
//
// 脚本以 PostgreSQL 编写；接入 MySQL 时新增一套等价脚本与同一套隔离用例。
package migrations

import "embed"

// FS 嵌入两套迁移脚本，供测试与 cmd/migrate 共用。
//
//go:embed platform/*.sql tenant/*.sql
var FS embed.FS

const (
	// PlatformDir 是平台组迁移目录。
	PlatformDir = "platform"
	// TenantDir 是租户组迁移目录。
	TenantDir = "tenant"
)
