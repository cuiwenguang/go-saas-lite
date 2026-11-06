package grant

import "time"

// Output 是一条授权记录。
type Output struct {
	ClientID   string     `json:"client_id"`
	ClientName string     `json:"client_name"`
	Scope      string     `json:"scope"`
	RevokedAt  *time.Time `json:"revoked_at"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}
