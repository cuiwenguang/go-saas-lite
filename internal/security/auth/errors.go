// Package auth 负责 JWT 的签发、校验与两条入口的 actor 隔离。
//
// 校验三件事（readme 2.2）：
//   - 签名与过期时间；
//   - aud：Web=tenant-api、Agent=mcp、平台=platform-api 不能互串；
//   - actor_type：平台链拒绝 tenant token，租户链拒绝 platform token。
package auth

import "errors"

// 入口对应的 audience。
const (
	AudienceTenantAPI   = "tenant-api"
	AudienceMCP         = "mcp"
	AudiencePlatformAPI = "platform-api"
)

// 校验错误，便于中间件映射错误码与单测断言。
var (
	ErrTokenMissing     = errors.New("auth: bearer token missing")
	ErrTokenInvalid     = errors.New("auth: token invalid")
	ErrTokenExpired     = errors.New("auth: token expired")
	ErrAudienceMismatch = errors.New("auth: audience mismatch")
	ErrActorMismatch    = errors.New("auth: actor type mismatch")
	ErrMissingTenant    = errors.New("auth: tenant token without tenant_id")
)
