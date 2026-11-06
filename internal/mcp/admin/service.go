package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/cuiwenguang/go-saas-lite/internal/mcp"
	"github.com/cuiwenguang/go-saas-lite/internal/security/authz"
	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// Service 负责上游 MCP 服务、工具的配置管理，以及调用日志查询。
type Service struct {
	db     tenancy.DB
	cipher *mcp.Cipher
}

// NewService 创建 MCP 管理服务。
func NewService(db tenancy.DB, cipher *mcp.Cipher) *Service {
	return &Service{db: db, cipher: cipher}
}

// ListServers 返回全部上游服务及工具数量。
func (s *Service) ListServers(ctx context.Context) ([]ServerOutput, error) {
	var servers []Server
	var tools []Tool

	if err := s.db.Read(ctx, func(tx tenancy.Tx) error {
		if err := tx.Order("created_at ASC").Find(&servers); err != nil {
			return err
		}

		return tx.Find(&tools)
	}); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "list mcp servers")
	}

	counts := make(map[string]int64, len(servers))
	for _, tool := range tools {
		counts[tool.ServerID]++
	}

	out := make([]ServerOutput, 0, len(servers))
	for _, server := range servers {
		out = append(out, toServerOutput(server, counts[server.ID]))
	}

	return out, nil
}

// CreateServer 新建上游服务；凭据加密后入库。
func (s *Service) CreateServer(ctx context.Context, in CreateServerInput) (*ServerOutput, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "name is required")
	}
	baseURL, err := normalizeBaseURL(in.BaseURL)
	if err != nil {
		return nil, err
	}

	credential, err := s.cipher.Encrypt(strings.TrimSpace(in.Credential))
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "encrypt credential")
	}

	server := Server{
		ID:         uuid.Must(uuid.NewV7()).String(),
		Name:       name,
		BaseURL:    baseURL,
		Credential: credential,
		Enabled:    in.Enabled == nil || *in.Enabled,
	}

	if err := s.db.Tx(ctx, func(tx tenancy.Tx) error {
		return tx.Create(&server)
	}); err != nil {
		return nil, conflictOrWrap(err, "create mcp server")
	}

	out := toServerOutput(server, 0)

	return &out, nil
}

// UpdateServer 修改上游服务；Credential 为空串表示清除。
func (s *Service) UpdateServer(ctx context.Context, id string, in UpdateServerInput) (*ServerOutput, error) {
	if _, err := s.server(ctx, id); err != nil {
		return nil, err
	}

	values := make(map[string]any, 4)
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return nil, apperr.New(apperr.CodeInvalidArgument, "name cannot be empty")
		}
		values["name"] = name
	}
	if in.BaseURL != nil {
		baseURL, err := normalizeBaseURL(*in.BaseURL)
		if err != nil {
			return nil, err
		}
		values["base_url"] = baseURL
	}
	if in.Credential != nil {
		credential, err := s.cipher.Encrypt(strings.TrimSpace(*in.Credential))
		if err != nil {
			return nil, apperr.Wrap(err, apperr.CodeInternal, "encrypt credential")
		}
		values["credential"] = credential
	}
	if in.Enabled != nil {
		values["enabled"] = *in.Enabled
	}

	if len(values) > 0 {
		if err := s.db.Tx(ctx, func(tx tenancy.Tx) error {
			return tx.Where("id = ?", id).Update(&Server{}, values)
		}); err != nil {
			return nil, conflictOrWrap(err, "update mcp server")
		}
	}

	server, err := s.server(ctx, id)
	if err != nil {
		return nil, err
	}
	var count int64
	if err := s.db.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Model(&Tool{}).Where("server_id = ?", id).Count(&count)
	}); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "count mcp tools")
	}

	out := toServerOutput(*server, count)

	return &out, nil
}

// DeleteServer 删除上游服务及其工具配置。
func (s *Service) DeleteServer(ctx context.Context, id string) error {
	if _, err := s.server(ctx, id); err != nil {
		return err
	}

	if err := s.db.Tx(ctx, func(tx tenancy.Tx) error {
		if err := tx.Where("server_id = ?", id).Delete(&Tool{}); err != nil {
			return err
		}

		return tx.Where("id = ?", id).Delete(&Server{})
	}); err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "delete mcp server")
	}

	return nil
}

// ListTools 返回工具配置，可按服务过滤。
func (s *Service) ListTools(ctx context.Context, serverID string) ([]ToolOutput, error) {
	var tools []Tool

	if err := s.db.Read(ctx, func(tx tenancy.Tx) error {
		query := tx.Model(&Tool{})
		if serverID != "" {
			query = query.Where("server_id = ?", serverID)
		}

		return query.Order("name ASC").Find(&tools)
	}); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "list mcp tools")
	}

	out := make([]ToolOutput, 0, len(tools))
	for _, tool := range tools {
		out = append(out, toToolOutput(tool))
	}

	return out, nil
}

// UpdateTool 只改启用状态与所需权限点，权限点必须已在代码里登记。
func (s *Service) UpdateTool(ctx context.Context, id string, in UpdateToolInput) (*ToolOutput, error) {
	var tool Tool
	err := s.db.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Where("id = ?", id).First(&tool)
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperr.New(apperr.CodeNotFound, "tool not found")
	}
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "load mcp tool")
	}

	values := make(map[string]any, 2)
	if in.Enabled != nil {
		values["enabled"] = *in.Enabled
	}
	if in.RequiredPerms != nil {
		perms, err := normalizePerms(*in.RequiredPerms)
		if err != nil {
			return nil, err
		}
		encoded, err := json.Marshal(perms)
		if err != nil {
			return nil, apperr.Wrap(err, apperr.CodeInternal, "encode required_perms")
		}
		values["required_perms"] = string(encoded)
	}

	if len(values) > 0 {
		if err := s.db.Tx(ctx, func(tx tenancy.Tx) error {
			return tx.Where("id = ?", id).Update(&Tool{}, values)
		}); err != nil {
			return nil, apperr.Wrap(err, apperr.CodeInternal, "update mcp tool")
		}
	}

	if err := s.db.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Where("id = ?", id).First(&tool)
	}); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "reload mcp tool")
	}

	out := toToolOutput(tool)

	return &out, nil
}

// ListCalls 分页返回调用日志，最新在前。
func (s *Service) ListCalls(ctx context.Context, filter CallFilter) ([]CallOutput, int64, error) {
	page, size := normalizePage(filter.Page, filter.Size)

	var rows []CallLog
	var total int64

	if err := s.db.Read(ctx, func(tx tenancy.Tx) error {
		query := tx.Model(&CallLog{})
		if status := strings.TrimSpace(filter.Status); status != "" {
			query = query.Where("status = ?", status)
		}
		if tool := strings.TrimSpace(filter.Tool); tool != "" {
			query = query.Where("tool_name = ?", tool)
		}
		if err := query.Count(&total); err != nil {
			return err
		}

		return query.Order("created_at DESC").Limit(size).Offset((page - 1) * size).Find(&rows)
	}); err != nil {
		return nil, 0, apperr.Wrap(err, apperr.CodeInternal, "list mcp calls")
	}

	out := make([]CallOutput, 0, len(rows))
	for _, row := range rows {
		out = append(out, CallOutput{
			ID:         row.ID,
			UserID:     row.UserID,
			ClientID:   row.ClientID,
			ToolName:   row.ToolName,
			Status:     row.Status,
			DurationMS: row.DurationMS,
			ErrorCode:  row.ErrorCode,
			CreatedAt:  row.CreatedAt,
		})
	}

	return out, total, nil
}

func (s *Service) server(ctx context.Context, id string) (*Server, error) {
	var server Server
	err := s.db.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Where("id = ?", id).First(&server)
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperr.New(apperr.CodeNotFound, "mcp server not found")
	}
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "load mcp server")
	}

	return &server, nil
}

func toServerOutput(server Server, toolCount int64) ServerOutput {
	return ServerOutput{
		ID:            server.ID,
		Name:          server.Name,
		BaseURL:       server.BaseURL,
		HasCredential: server.Credential != "",
		Enabled:       server.Enabled,
		ToolCount:     toolCount,
		CreatedAt:     server.CreatedAt,
		UpdatedAt:     server.UpdatedAt,
	}
}

func toToolOutput(tool Tool) ToolOutput {
	perms := make([]string, 0)
	if strings.TrimSpace(tool.RequiredPerms) != "" {
		_ = json.Unmarshal([]byte(tool.RequiredPerms), &perms)
	}

	schema := json.RawMessage(tool.InputSchema)
	if len(schema) == 0 {
		schema = json.RawMessage(`{}`)
	}

	return ToolOutput{
		ID:            tool.ID,
		ServerID:      tool.ServerID,
		Name:          tool.Name,
		Description:   tool.Description,
		InputSchema:   schema,
		RequiredPerms: perms,
		Enabled:       tool.Enabled,
		UpdatedAt:     tool.UpdatedAt,
	}
}

// normalizeBaseURL 只校验协议与主机；运行期还有 SSRF 校验（内网地址拦截）。
func normalizeBaseURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", apperr.New(apperr.CodeInvalidArgument, "base_url must be an http(s) url")
	}

	return trimmed, nil
}

// normalizePerms 去重并校验权限点已登记，避免写出无法生效的配置。
func normalizePerms(codes []string) ([]string, error) {
	known := make(map[string]struct{})
	for _, perm := range authz.All() {
		known[perm.String()] = struct{}{}
	}

	out := make([]string, 0, len(codes))
	seen := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		if _, ok := known[code]; !ok {
			return nil, apperr.New(apperr.CodeInvalidArgument, "unknown permission: "+code)
		}
		if _, dup := seen[code]; dup {
			continue
		}
		seen[code] = struct{}{}
		out = append(out, code)
	}

	return out, nil
}

func normalizePage(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}

	return page, size
}

func conflictOrWrap(err error, action string) error {
	if strings.Contains(err.Error(), "23505") {
		return apperr.New(apperr.CodeConflict, "mcp server name already exists")
	}

	return apperr.Wrap(err, apperr.CodeInternal, action)
}
