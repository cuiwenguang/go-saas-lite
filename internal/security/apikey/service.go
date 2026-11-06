package apikey

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// Service 读写 api_keys。
type Service struct {
	platform tenancy.PlatformDB
	now      func() time.Time
}

// NewService 创建服务。
func NewService(platform tenancy.PlatformDB) *Service {
	return &Service{platform: platform, now: time.Now}
}

// Create 生成 Key，明文只返回一次。
func (s *Service) Create(ctx context.Context, tenantID, userID, name string) (string, *Key, error) {
	if tenantID == "" || userID == "" {
		return "", nil, apperr.New(apperr.CodeInvalidArgument, "tenant and user are required")
	}
	if strings.TrimSpace(name) == "" {
		return "", nil, apperr.New(apperr.CodeInvalidArgument, "name is required")
	}

	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, apperr.Wrap(err, apperr.CodeInternal, "generate api key")
	}
	plain := KeyPrefix + base64.RawURLEncoding.EncodeToString(raw)

	row := &APIKey{
		ID:        uuid.Must(uuid.NewV7()).String(),
		TenantID:  tenantID,
		UserID:    userID,
		Name:      name,
		KeyPrefix: plain[:len(KeyPrefix)+6],
		KeyHash:   hashKey(plain),
	}

	if err := s.platform.Tx(ctx, func(tx tenancy.Tx) error {
		return tx.Create(row)
	}); err != nil {
		return "", nil, apperr.Wrap(err, apperr.CodeInternal, "create api key")
	}

	return plain, toKey(row), nil
}

// List 列出用户自己的 Key。
func (s *Service) List(ctx context.Context, tenantID, userID string) ([]Key, error) {
	var rows []APIKey
	err := s.platform.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Where("tenant_id = ? AND user_id = ?", tenantID, userID).
			Order("created_at DESC").
			Find(&rows)
	})
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "list api keys")
	}

	keys := make([]Key, 0, len(rows))
	for i := range rows {
		keys = append(keys, *toKey(&rows[i]))
	}

	return keys, nil
}

// Revoke 撤销自己的 Key。
func (s *Service) Revoke(ctx context.Context, tenantID, userID, id string) error {
	err := s.platform.Tx(ctx, func(tx tenancy.Tx) error {
		return tx.Where("id = ? AND tenant_id = ? AND user_id = ?", id, tenantID, userID).
			Update(&APIKey{}, map[string]any{"revoked_at": s.now()})
	})
	if err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "revoke api key")
	}

	return nil
}

// Authenticate 用明文 Key 换取身份；同一请求内撤销立即生效。
func (s *Service) Authenticate(ctx context.Context, plain string) (Identity, error) {
	if !strings.HasPrefix(plain, KeyPrefix) {
		return Identity{}, apperr.New(apperr.CodeInvalidToken, "invalid api key")
	}

	var row APIKey
	err := s.platform.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Where("key_hash = ?", hashKey(plain)).First(&row)
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Identity{}, apperr.New(apperr.CodeInvalidToken, "invalid api key")
	}
	if err != nil {
		return Identity{}, apperr.Wrap(err, apperr.CodeInternal, "authenticate api key")
	}
	if row.RevokedAt != nil {
		return Identity{}, apperr.New(apperr.CodeInvalidToken, "api key revoked")
	}

	_ = s.platform.Tx(ctx, func(tx tenancy.Tx) error {
		return tx.Where("id = ?", row.ID).Update(&APIKey{}, map[string]any{"last_used_at": s.now()})
	})

	return Identity{TenantID: row.TenantID, UserID: row.UserID, ClientID: fmt.Sprintf("apikey:%s", row.ID)}, nil
}

func hashKey(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

func toKey(row *APIKey) *Key {
	return &Key{
		ID:         row.ID,
		Name:       row.Name,
		KeyPrefix:  row.KeyPrefix,
		RevokedAt:  row.RevokedAt,
		LastUsedAt: row.LastUsedAt,
		CreatedAt:  row.CreatedAt,
	}
}
