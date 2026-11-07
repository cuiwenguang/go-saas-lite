package grant

import (
	"context"
	"time"

	"github.com/cuiwenguang/go-saas-lite/internal/mcp"
	auditlog "github.com/cuiwenguang/go-saas-lite/internal/tenant/audit"
	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// Service 管理当前用户的 OAuth 授权记录。
type Service struct {
	db       tenancy.DB
	platform tenancy.PlatformDB
	sessions mcp.SessionStore
	audit    *auditlog.Service
}

// NewService 创建授权服务；sessions / audit 为 nil 时对应能力关闭。
func NewService(db tenancy.DB, platform tenancy.PlatformDB, sessions mcp.SessionStore, auditService *auditlog.Service) *Service {
	return &Service{db: db, platform: platform, sessions: sessions, audit: auditService}
}

// List 返回当前用户的授权；includeRevoked 为真时含已撤销记录。
func (s *Service) List(ctx context.Context, includeRevoked bool) ([]Output, error) {
	scope, err := tenancy.ScopeFrom(ctx)
	if err != nil || scope.UserID == "" {
		return nil, apperr.New(apperr.CodeUnauthenticated, "user identity required")
	}

	var rows []Grant
	if err := s.db.Read(ctx, func(tx tenancy.Tx) error {
		query := tx.Model(&Grant{}).Where("user_id = ?", scope.UserID)
		if !includeRevoked {
			query = query.Where("revoked_at IS NULL")
		}

		return query.Order("created_at DESC").Find(&rows)
	}); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "list oauth grants")
	}

	names := make(map[string]string, len(rows))
	if len(rows) > 0 {
		ids := make([]string, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.ClientID)
		}

		var clients []Client
		if err := s.platform.Read(ctx, func(tx tenancy.Tx) error {
			return tx.Model(&Client{}).Where("client_id IN ?", ids).Find(&clients)
		}); err != nil {
			return nil, apperr.Wrap(err, apperr.CodeInternal, "load oauth clients")
		}
		for _, client := range clients {
			names[client.ClientID] = client.ClientName
		}
	}

	out := make([]Output, 0, len(rows))
	for _, row := range rows {
		name := names[row.ClientID]
		if name == "" {
			name = row.ClientID
		}
		out = append(out, Output{
			ClientID:   row.ClientID,
			ClientName: name,
			Scope:      row.Scope,
			RevokedAt:  row.RevokedAt,
			CreatedAt:  row.CreatedAt,
			UpdatedAt:  row.UpdatedAt,
		})
	}

	return out, nil
}

// Revoke 撤销当前用户对指定客户端的授权：刷新令牌立即失效，访问令牌到期后失效。
func (s *Service) Revoke(ctx context.Context, clientID string) error {
	scope, err := tenancy.ScopeFrom(ctx)
	if err != nil || scope.UserID == "" {
		return apperr.New(apperr.CodeUnauthenticated, "user identity required")
	}

	var affected int64
	if err := s.db.Tx(ctx, func(tx tenancy.Tx) error {
		count := int64(0)
		if err := tx.Model(&Grant{}).
			Where("user_id = ? AND client_id = ? AND revoked_at IS NULL", scope.UserID, clientID).
			Count(&count); err != nil {
			return err
		}
		if count == 0 {
			return nil
		}
		affected = count

		return tx.Where("user_id = ? AND client_id = ?", scope.UserID, clientID).
			Update(&Grant{}, map[string]any{"revoked_at": time.Now()})
	}); err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "revoke oauth grant")
	}
	if affected == 0 {
		return apperr.New(apperr.CodeNotFound, "authorization not found")
	}

	// 授权已撤销，会话作废失败不回滚数据库状态（readme 2.5）。
	if s.sessions != nil {
		_ = s.sessions.RevokeClient(ctx, scope.TenantID, scope.UserID, clientID)
	}

	if s.audit != nil {
		_ = s.audit.Record(ctx, auditlog.Entry{
			Category:   auditlog.CategoryOAuth,
			Action:     auditlog.ActionOAuthGrantRevoke,
			Level:      auditlog.LevelWarn,
			ObjectType: "oauth_client",
			ObjectID:   clientID,
			Summary:    "撤销 OAuth 授权 " + clientID,
		})
	}

	return nil
}
