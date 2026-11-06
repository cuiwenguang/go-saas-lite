package conf

import "time"

// Entry 是一条配置。
type Entry struct {
	Namespace string    `json:"namespace"`
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SetConfigInput 是写入 / 更新配置的入参。
type SetConfigInput struct {
	Value string `json:"value"`
}
