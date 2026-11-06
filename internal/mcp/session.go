package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Context 是一次调用的身份上下文，避免业务包依赖 HTTP。
type Context struct {
	Ctx      context.Context
	TenantID string
	UserID   string
	ClientID string
}

// Session 是 MCP 会话，绑定租户、用户与客户端。
type Session struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	UserID    string    `json:"user_id"`
	ClientID  string    `json:"client_id"`
	CreatedAt time.Time `json:"created_at"`
}

// SessionStore 保存会话；默认 Redis，未配置时用内存实现。
type SessionStore interface {
	Create(ctx context.Context, session Session) error
	Get(ctx context.Context, tenantID, id string) (Session, error)
	Delete(ctx context.Context, tenantID, id string) error
}

// ErrSessionNotFound 表示会话不存在或已过期。
var ErrSessionNotFound = errors.New("mcp: session not found")

// RedisSessions 把会话放在 Redis，任意副本都能处理。
type RedisSessions struct {
	client redis.UniversalClient
	ttl    time.Duration
}

// NewRedisSessions 创建 Redis 会话存储。
func NewRedisSessions(client redis.UniversalClient, ttl time.Duration) *RedisSessions {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	return &RedisSessions{client: client, ttl: ttl}
}

func sessionKey(tenantID, id string) string {
	return fmt.Sprintf("mcp:session:%s:%s", tenantID, id)
}

func (s *RedisSessions) Create(ctx context.Context, session Session) error {
	payload, err := json.Marshal(session)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, sessionKey(session.TenantID, session.ID), payload, s.ttl).Err()
}

func (s *RedisSessions) Get(ctx context.Context, tenantID, id string) (Session, error) {
	raw, err := s.client.Get(ctx, sessionKey(tenantID, id)).Bytes()
	if errors.Is(err, redis.Nil) {
		return Session{}, ErrSessionNotFound
	}
	if err != nil {
		return Session{}, err
	}

	var session Session
	if err := json.Unmarshal(raw, &session); err != nil {
		return Session{}, err
	}

	return session, nil
}

func (s *RedisSessions) Delete(ctx context.Context, tenantID, id string) error {
	return s.client.Del(ctx, sessionKey(tenantID, id)).Err()
}

// MemorySessions 是单机会话存储，仅用于未启用 Redis 的开发环境。
type MemorySessions struct {
	sessions map[string]Session
}

// NewMemorySessions 创建内存会话存储。
func NewMemorySessions() *MemorySessions {
	return &MemorySessions{sessions: map[string]Session{}}
}

func memoryKey(tenantID, id string) string { return tenantID + "|" + id }

// Create 保存会话。
func (m *MemorySessions) Create(_ context.Context, session Session) error {
	m.sessions[memoryKey(session.TenantID, session.ID)] = session
	return nil
}

// Get 读取会话。
func (m *MemorySessions) Get(_ context.Context, tenantID, id string) (Session, error) {
	session, ok := m.sessions[memoryKey(tenantID, id)]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	return session, nil
}

// Delete 删除会话。
func (m *MemorySessions) Delete(_ context.Context, tenantID, id string) error {
	delete(m.sessions, memoryKey(tenantID, id))
	return nil
}

// NewSessionID 生成会话标识。
func NewSessionID() string {
	return uuid.Must(uuid.NewV7()).String()
}
