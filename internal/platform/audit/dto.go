package audit

import (
	"encoding/json"
	"time"
)

// Entry 是一次审计写入；Before / After 由调用方给出可序列化的结构。
type Entry struct {
	Category   string
	Action     string
	Level      string
	Result     string
	ObjectType string
	ObjectID   string
	Summary    string
	Before     any
	After      any
	// 以下三项用于没有 Scope 的场景（如登录失败）显式指定操作人。
	ActorType  string
	ActorID    string
	ActorEmail string
}

// Filter 是审计查询条件。
type Filter struct {
	Category string
	Action   string
	Level    string
	Actor    string
	ObjectID string
	From     *time.Time
	To       *time.Time
	Page     int
	Size     int
}

// Output 是审计列表出参；before / after 直接返回 JSON 对象，前端做 diff。
type Output struct {
	ID         string          `json:"id"`
	Category   string          `json:"category"`
	Action     string          `json:"action"`
	Level      string          `json:"level"`
	Result     string          `json:"result"`
	ActorType  string          `json:"actor_type"`
	ActorEmail string          `json:"actor_email,omitempty"`
	ObjectType string          `json:"object_type"`
	ObjectID   string          `json:"object_id"`
	Summary    string          `json:"summary"`
	Before     json.RawMessage `json:"before,omitempty"`
	After      json.RawMessage `json:"after,omitempty"`
	RequestID  string          `json:"request_id,omitempty"`
	IP         string          `json:"ip,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
}
