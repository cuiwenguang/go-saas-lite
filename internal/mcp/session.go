package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
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
	// Count 返回租户当前活跃会话数，供并发配额校验使用。
	Count(ctx context.Context, tenantID string) (int64, error)
	// RevokeClient 作废某用户在某客户端下的全部会话（撤销授权时调用）。
	RevokeClient(ctx context.Context, tenantID, userID, clientID string) error
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

func sessionSetKey(tenantID string) string {
	return "mcp:sessions:" + tenantID
}

func sessionUserKey(tenantID, userID, clientID string) string {
	return fmt.Sprintf("mcp:sessions:user:%s:%s:%s", tenantID, userID, clientID)
}

func (s *RedisSessions) Create(ctx context.Context, session Session) error {
	payload, err := json.Marshal(session)
	if err != nil {
		return err
	}
	if err := s.client.Set(ctx, sessionKey(session.TenantID, session.ID), payload, s.ttl).Err(); err != nil {
		return err
	}

	// 用 ZSET 维护活跃会话，score 为过期时间，避免逐 key SCAN。
	setKey := sessionSetKey(session.TenantID)
	if err := s.client.ZAdd(ctx, setKey, redis.Z{
		Score:  float64(time.Now().Add(s.ttl).Unix()),
		Member: session.ID,
	}).Err(); err != nil {
		return err
	}

	if err := s.client.Expire(ctx, setKey, s.ttl*2).Err(); err != nil {
		return err
	}

	userKey := sessionUserKey(session.TenantID, session.UserID, session.ClientID)
	if err := s.client.SAdd(ctx, userKey, session.ID).Err(); err != nil {
		return err
	}

	return s.client.Expire(ctx, userKey, s.ttl*2).Err()
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
	session, err := s.Get(ctx, tenantID, id)
	if err == nil {
		_ = s.client.SRem(ctx, sessionUserKey(tenantID, session.UserID, session.ClientID), id).Err()
	}

	if err := s.client.Del(ctx, sessionKey(tenantID, id)).Err(); err != nil {
		return err
	}

	return s.client.ZRem(ctx, sessionSetKey(tenantID), id).Err()
}

func (s *RedisSessions) Count(ctx context.Context, tenantID string) (int64, error) {
	setKey := sessionSetKey(tenantID)
	if err := s.client.ZRemRangeByScore(ctx, setKey, "-inf", strconv.FormatInt(time.Now().Unix(), 10)).Err(); err != nil {
		return 0, err
	}

	return s.client.ZCard(ctx, setKey).Result()
}

func (s *RedisSessions) RevokeClient(ctx context.Context, tenantID, userID, clientID string) error {
	userKey := sessionUserKey(tenantID, userID, clientID)
	ids, err := s.client.SMembers(ctx, userKey).Result()
	if err != nil {
		return err
	}

	for _, id := range ids {
		_ = s.client.Del(ctx, sessionKey(tenantID, id)).Err()
		_ = s.client.ZRem(ctx, sessionSetKey(tenantID), id).Err()
	}

	return s.client.Del(ctx, userKey).Err()
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

func (m *MemorySessions) Count(_ context.Context, tenantID string) (int64, error) {
	prefix := tenantID + "|"
	var count int64
	for key := range m.sessions {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			count++
		}
	}

	return count, nil
}

func (m *MemorySessions) RevokeClient(_ context.Context, tenantID, userID, clientID string) error {
	for key, session := range m.sessions {
		if session.TenantID == tenantID && session.UserID == userID && session.ClientID == clientID {
			delete(m.sessions, key)
		}
	}

	return nil
}

// NewSessionID 生成会话标识。
func NewSessionID() string {
	return uuid.Must(uuid.NewV7()).String()
}
