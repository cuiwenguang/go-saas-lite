package mcp

import (
	"encoding/json"
	"time"
)

// JSON-RPC 2.0 错误码（readme 1.11）。
const (
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeForbidden      = -32003
	CodeUpstream       = -32000
	CodeQuotaExceeded  = -32001
)

// Request 是入站 JSON-RPC 请求。
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// RPCError 是错误体；平台错误码放在 Data.code。
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Response 是出站响应。
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// Tool 是工具定义。
type Tool struct {
	Name          string          `json:"name"`
	Description   string          `json:"description,omitempty"`
	InputSchema   json.RawMessage `json:"inputSchema,omitempty"`
	RequiredPerms []string        `json:"-"`
}

// Provider 提供工具列表与执行。
type Provider interface {
	List(ctx Context) ([]Tool, error)
	Call(ctx Context, name string, args json.RawMessage) (json.RawMessage, error)
}

// CallRecord 是一次工具调用的审计记录。
type CallRecord struct {
	TenantID   string
	UserID     string
	ClientID   string
	ToolName   string
	Status     string
	DurationMS int64
	ErrorCode  string
	CreatedAt  time.Time
}

// CallLogger 异步写调用日志。
type CallLogger interface {
	Log(ctx Context, record CallRecord)
}
