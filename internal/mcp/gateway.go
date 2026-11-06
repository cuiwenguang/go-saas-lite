// Package mcp 实现 MCP 协议、会话、工具网关与调用日志。
package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// Gateway 实现 Provider 接口：平台内置工具 + 租户注册的工具。
type Gateway struct {
	db            tenancy.DB
	client        *http.Client
	allowLoopback bool
}

// NewGateway 创建工具网关。
func NewGateway(db tenancy.DB) *Gateway {
	return &Gateway{db: db, client: &http.Client{Timeout: 30 * time.Second}}
}

// WithLoopback 允许上游指向回环地址（仅供测试）。
func (g *Gateway) WithLoopback() *Gateway {
	g.allowLoopback = true
	return g
}

// builtinTools 是平台自有工具，不依赖 mcp_tools 配置。
var builtinTools = []Tool{
	{
		Name:          "tenant.info",
		Description:   "返回当前租户与调用者身份",
		InputSchema:   json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		RequiredPerms: nil,
	},
}

// List 返回内置工具与租户启用的工具。
func (g *Gateway) List(ctx Context) ([]Tool, error) {
	tools := make([]Tool, 0, len(builtinTools)+4)
	tools = append(tools, builtinTools...)

	var rows []ToolRecord
	err := g.db.Read(ctx.Ctx, func(tx tenancy.Tx) error {
		return tx.Where("enabled = ?", true).Find(&rows)
	})
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "list mcp tools")
	}

	for _, row := range rows {
		perms, parseErr := parsePerms(row.RequiredPerms)
		if parseErr != nil {
			return nil, parseErr
		}
		tools = append(tools, Tool{
			Name:          row.Name,
			Description:   row.Description,
			InputSchema:   json.RawMessage(row.InputSchema),
			RequiredPerms: perms,
		})
	}

	return tools, nil
}

// Call 执行工具：内置工具直接处理，其余转发到上游 MCP 服务。
func (g *Gateway) Call(ctx Context, name string, args json.RawMessage) (json.RawMessage, error) {
	switch name {
	case "tenant.info":
		payload, err := json.Marshal(map[string]string{
			"tenant_id": ctx.TenantID,
			"user_id":   ctx.UserID,
			"client_id": ctx.ClientID,
		})
		if err != nil {
			return nil, err
		}
		return payload, nil
	default:
		return g.proxyCall(ctx.Ctx, name, args)
	}
}

func parsePerms(raw string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}

	var perms []string
	if err := json.Unmarshal([]byte(raw), &perms); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "parse required_perms")
	}
	return perms, nil
}

// CallLog 异步写 mcp_call_log，不阻塞响应。
type CallLog struct {
	db tenancy.DB
}

// NewCallLog 创建日志写入器。
func NewCallLog(db tenancy.DB) *CallLog {
	return &CallLog{db: db}
}

// Log 在独立上下文里写入，避免请求取消后丢日志。
func (l *CallLog) Log(_ Context, record CallRecord) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		ctx = tenancy.WithScope(ctx, tenancy.Scope{
			TenantID: record.TenantID,
			UserID:   record.UserID,
			ClientID: record.ClientID,
			Actor:    tenancy.ActorTenant,
		})

		row := &CallLogRecord{
			ID:         uuid.Must(uuid.NewV7()).String(),
			UserID:     record.UserID,
			ClientID:   record.ClientID,
			ToolName:   record.ToolName,
			Status:     record.Status,
			DurationMS: record.DurationMS,
			ErrorCode:  record.ErrorCode,
			CreatedAt:  record.CreatedAt,
		}

		if err := l.db.Tx(ctx, func(tx tenancy.Tx) error {
			return tx.Create(row)
		}); err != nil && err != gorm.ErrRecordNotFound {
			_ = err
		}
	}()
}
