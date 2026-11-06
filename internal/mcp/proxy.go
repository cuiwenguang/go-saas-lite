package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"

	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// proxyCall 把工具调用转发到租户配置的上游 MCP 服务。
func (g *Gateway) proxyCall(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	var tool ToolRecord
	err := g.db.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Where("name = ? AND enabled = ?", name, true).First(&tool)
	})
	if err != nil {
		return nil, fmt.Errorf("tool %q is not executable in this build", name)
	}

	var server Server
	err = g.db.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Where("id = ? AND enabled = ?", tool.ServerID, true).First(&server)
	})
	if err != nil {
		return nil, fmt.Errorf("upstream server for tool %q is unavailable", name)
	}

	if err := validateUpstream(server.BaseURL, g.allowLoopback); err != nil {
		return nil, err
	}

	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      "1",
		"method":  "tools/call",
		"params": map[string]any{
			"name":      name,
			"arguments": json.RawMessage(args),
		},
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.BaseURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if server.Credential != "" {
		credential, err := g.cipher.Decrypt(server.Credential)
		if err != nil {
			return nil, fmt.Errorf("decrypt upstream credential: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+credential)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call upstream: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read upstream response: %w", err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("upstream returned %d", resp.StatusCode)
	}

	var rpc struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &rpc); err != nil {
		return nil, fmt.Errorf("decode upstream response: %w", err)
	}
	if rpc.Error != nil {
		return nil, fmt.Errorf("upstream error %d: %s", rpc.Error.Code, rpc.Error.Message)
	}
	if len(rpc.Result) == 0 {
		return nil, fmt.Errorf("upstream returned empty result")
	}

	return rpc.Result, nil
}

// validateUpstream 拒绝私网 / 回环 / 链路本地地址，防止 SSRF。
func validateUpstream(raw string, allowLoopback bool) error {
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return apperr.New(apperr.CodeInvalidArgument, "invalid upstream url")
	}
	if allowLoopback {
		return nil
	}

	host := parsed.Hostname()
	if host == "" {
		return apperr.New(apperr.CodeInvalidArgument, "invalid upstream host")
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		return apperr.New(apperr.CodeInvalidArgument, "cannot resolve upstream host")
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return apperr.New(apperr.CodeInvalidArgument, "upstream address is not allowed")
		}
	}

	return nil
}
