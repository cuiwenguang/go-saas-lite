package apikey

import "time"

// Key 是对外展示的 Key 信息（不含明文）。
type Key struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	KeyPrefix  string     `json:"key_prefix"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Identity 是鉴权结果。
type Identity struct {
	TenantID string
	UserID   string
	ClientID string // apikey:{id}
}
