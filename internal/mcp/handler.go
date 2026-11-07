package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cuiwenguang/go-saas-lite/internal/quota"
	"github.com/cuiwenguang/go-saas-lite/internal/security/authz"
	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// SessionHeader 是会话标识头。
const SessionHeader = "Mcp-Session-Id"

// Handler 实现 MCP Streamable HTTP 单端点。
type Handler struct {
	provider        Provider
	authorizer      authz.Authorizer
	quota           *quota.Service
	sessions        SessionStore
	calls           CallLogger
	serverName      string
	serverVersion   string
	protocolVersion string
}

// Options 配置 MCP handler。
type Options struct {
	Provider        Provider
	Authorizer      authz.Authorizer
	Quota           *quota.Service
	Sessions        SessionStore
	Calls           CallLogger
	ServerName      string
	ServerVersion   string
	ProtocolVersion string
}

// NewHandler 创建 MCP handler。
func NewHandler(opts Options) *Handler {
	if opts.ServerName == "" {
		opts.ServerName = "go-saas-lite"
	}
	if opts.ServerVersion == "" {
		opts.ServerVersion = "1.0.0"
	}
	if opts.ProtocolVersion == "" {
		opts.ProtocolVersion = "2025-06-18"
	}

	return &Handler{
		provider:        opts.Provider,
		authorizer:      opts.Authorizer,
		quota:           opts.Quota,
		sessions:        opts.Sessions,
		calls:           opts.Calls,
		serverName:      opts.ServerName,
		serverVersion:   opts.ServerVersion,
		protocolVersion: opts.ProtocolVersion,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	scope, err := tenancy.ScopeFrom(r.Context())
	if err != nil {
		writeRPCError(w, nil, RPCError{Code: CodeForbidden, Message: "missing tenant scope"})
		return
	}

	body, readErr := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if readErr != nil {
		writeRPCError(w, nil, RPCError{Code: CodeInvalidRequest, Message: "read body failed"})
		return
	}

	// 客户端要求流式时按 SSE 逐帧返回（单帧响应）。
	if strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		w = &sseWriter{ResponseWriter: w}
	}

	var request Request
	if err := json.Unmarshal(body, &request); err != nil || request.Method == "" {
		writeRPCError(w, nil, RPCError{Code: CodeInvalidRequest, Message: "invalid jsonrpc request"})
		return
	}

	callCtx := Context{Ctx: r.Context(), TenantID: scope.TenantID, UserID: scope.UserID, ClientID: scope.ClientID}

	switch request.Method {
	case "initialize":
		h.initialize(w, callCtx, request)
	case "ping":
		writeRPCResult(w, request.ID, map[string]any{})
	case "tools/list":
		if !h.requireSession(w, r, request, callCtx) {
			return
		}
		h.listTools(w, callCtx, request)
	case "tools/call":
		if !h.requireSession(w, r, request, callCtx) {
			return
		}
		h.callTool(w, callCtx, request)
	default:
		writeRPCError(w, request.ID, RPCError{Code: CodeMethodNotFound, Message: "method not found"})
	}
}

func (h *Handler) initialize(w http.ResponseWriter, callCtx Context, request Request) {
	if h.quota != nil {
		count, err := h.sessions.Count(callCtx.Ctx, callCtx.TenantID)
		if err != nil {
			writeRPCError(w, request.ID, RPCError{Code: CodeUpstream, Message: "count sessions failed"})
			return
		}
		if err := h.quota.CheckCount(callCtx.Ctx, callCtx.TenantID, quota.MetricMCPSessions, count); err != nil {
			writeRPCError(w, request.ID, RPCError{
				Code:    CodeQuotaExceeded,
				Message: err.Error(),
				Data:    errorData(err),
			})
			return
		}
	}

	session := Session{
		ID:        NewSessionID(),
		TenantID:  callCtx.TenantID,
		UserID:    callCtx.UserID,
		ClientID:  callCtx.ClientID,
		CreatedAt: time.Now(),
	}
	if err := h.sessions.Create(callCtx.Ctx, session); err != nil {
		writeRPCError(w, request.ID, RPCError{Code: CodeUpstream, Message: "create session failed"})
		return
	}

	w.Header().Set(SessionHeader, session.ID)
	writeRPCResult(w, request.ID, map[string]any{
		"protocolVersion": h.protocolVersion,
		"capabilities": map[string]any{
			"tools": map[string]any{},
		},
		"serverInfo": map[string]any{
			"name":    h.serverName,
			"version": h.serverVersion,
		},
	})
}

// requireSession 校验会话与当前身份一致；不一致按 readme 返回 404。
func (h *Handler) requireSession(w http.ResponseWriter, r *http.Request, request Request, callCtx Context) bool {
	sessionID := r.Header.Get(SessionHeader)
	if sessionID == "" {
		writeRPCError(w, request.ID, RPCError{Code: CodeInvalidRequest, Message: "missing Mcp-Session-Id"})
		return false
	}

	session, err := h.sessions.Get(callCtx.Ctx, callCtx.TenantID, sessionID)
	if err != nil ||
		session.TenantID != callCtx.TenantID ||
		session.UserID != callCtx.UserID ||
		session.ClientID != callCtx.ClientID {
		http.Error(w, "session not found", http.StatusNotFound)
		return false
	}

	return true
}

func (h *Handler) listTools(w http.ResponseWriter, callCtx Context, request Request) {
	tools, err := h.provider.List(callCtx)
	if err != nil {
		writeRPCError(w, request.ID, RPCError{Code: CodeUpstream, Message: "list tools failed"})
		return
	}

	visible := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		if !h.allowed(callCtx, tool.RequiredPerms) {
			continue
		}
		visible = append(visible, map[string]any{
			"name":        tool.Name,
			"description": tool.Description,
			"inputSchema": tool.InputSchema,
		})
	}

	writeRPCResult(w, request.ID, map[string]any{"tools": visible})
}

func (h *Handler) callTool(w http.ResponseWriter, callCtx Context, request Request) {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(request.Params, &params); err != nil || params.Name == "" {
		writeRPCError(w, request.ID, RPCError{Code: CodeInvalidParams, Message: "tool name is required"})
		return
	}

	tools, err := h.provider.List(callCtx)
	if err != nil {
		writeRPCError(w, request.ID, RPCError{Code: CodeUpstream, Message: "list tools failed"})
		return
	}

	var target *Tool
	for i := range tools {
		if tools[i].Name == params.Name {
			target = &tools[i]
			break
		}
	}
	if target == nil {
		writeRPCError(w, request.ID, RPCError{Code: CodeMethodNotFound, Message: "tool not found"})
		return
	}
	if !h.allowed(callCtx, target.RequiredPerms) {
		writeRPCError(w, request.ID, RPCError{Code: CodeForbidden, Message: "tool forbidden"})
		return
	}

	if err := validateInput(target.InputSchema, params.Arguments); err != nil {
		writeRPCError(w, request.ID, RPCError{
			Code:    CodeInvalidParams,
			Message: "invalid arguments: " + err.Error(),
		})
		h.record(callCtx, params.Name, "failed", 0, "invalid_params")
		return
	}

	if h.quota != nil {
		if quotaErr := h.quota.Consume(callCtx.Ctx, callCtx.TenantID, quota.MetricMCPCalls, 1); quotaErr != nil {
			code := CodeUpstream
			if apperr.Is(quotaErr, apperr.CodeQuotaExceeded) {
				code = CodeQuotaExceeded
			}
			writeRPCError(w, request.ID, RPCError{Code: code, Message: quotaErr.Error(), Data: errorData(quotaErr)})
			h.record(callCtx, params.Name, "failed", 0, "quota")
			return
		}

		// 先看当日 token 额度是否已用尽，具体消耗在调用成功后计量。
		if quotaErr := h.quota.Check(callCtx.Ctx, callCtx.TenantID, quota.MetricMCPToken, 1); quotaErr != nil {
			writeRPCError(w, request.ID, RPCError{
				Code:    CodeQuotaExceeded,
				Message: quotaErr.Error(),
				Data:    errorData(quotaErr),
			})
			h.record(callCtx, params.Name, "failed", 0, "quota")
			return
		}
	}

	start := time.Now()
	result, callErr := h.provider.Call(callCtx, params.Name, params.Arguments)
	duration := time.Since(start).Milliseconds()

	if callErr != nil {
		writeRPCError(w, request.ID, RPCError{Code: CodeUpstream, Message: callErr.Error(), Data: errorData(callErr)})
		h.record(callCtx, params.Name, "failed", duration, "upstream")
		return
	}

	if h.quota != nil {
		// MCP 协议不返回 token 用量，按响应字节数估算（4 字节≈1 token）。
		tokens := int64((len(result) + 3) / 4)
		if tokens > 0 {
			_ = h.quota.Incr(callCtx.Ctx, callCtx.TenantID, quota.MetricMCPToken, tokens)
		}
	}

	h.record(callCtx, params.Name, "succeeded", duration, "")
	writeRPCResult(w, request.ID, map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": string(result)},
		},
	})
}

func (h *Handler) allowed(callCtx Context, perms []string) bool {
	for _, perm := range perms {
		obj, act, ok := strings.Cut(perm, ":")
		if !ok {
			return false
		}
		if err := h.authorizer.Enforce(callCtx.Ctx, obj, act); err != nil {
			return false
		}
	}
	return true
}

func (h *Handler) record(callCtx Context, tool, status string, durationMS int64, errorCode string) {
	if h.calls == nil {
		return
	}
	h.calls.Log(callCtx, CallRecord{
		TenantID:   callCtx.TenantID,
		UserID:     callCtx.UserID,
		ClientID:   callCtx.ClientID,
		ToolName:   tool,
		Status:     status,
		DurationMS: durationMS,
		ErrorCode:  errorCode,
		CreatedAt:  time.Now(),
	})
}

func errorData(err error) map[string]any {
	if appErr := apperr.From(err); appErr != nil && appErr.Code != apperr.CodeInternal {
		return map[string]any{"code": fmt.Sprintf("%d", appErr.Code)}
	}
	return nil
}

func writeRPCResult(w http.ResponseWriter, id json.RawMessage, result any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(Response{JSONRPC: "2.0", ID: id, Result: result})
}

func writeRPCError(w http.ResponseWriter, id json.RawMessage, rpcErr RPCError) {
	status := http.StatusOK
	if rpcErr.Code == CodeForbidden {
		status = http.StatusForbidden
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Response{JSONRPC: "2.0", ID: id, Error: &rpcErr})
}

// sseWriter 把 JSON 响应包装成一条 SSE 消息帧。
type sseWriter struct {
	http.ResponseWriter
}

func (s *sseWriter) Write(p []byte) (int, error) {
	s.Header().Set("Content-Type", "text/event-stream")
	frame := make([]byte, 0, len(p)+16)
	frame = append(frame, "event: message\ndata: "...)
	frame = append(frame, p...)
	frame = append(frame, '\n', '\n')

	if _, err := s.ResponseWriter.Write(frame); err != nil {
		return 0, err
	}

	return len(p), nil
}
