// Package quota 负责套餐配额校验与用量计数（readme 5）。
//
// 限额来自 `plans.quota`（平台组），用量按天计在 Redis 上，
// 由 worker 定期落到 `usage_daily`。
package quota

import (
	"encoding/json"
	"fmt"
)

// Metric 是配额指标。
type Metric string

const (
	MetricAPICalls Metric = "api_calls"
	MetricMCPCalls Metric = "mcp_calls"
	MetricMCPToken Metric = "mcp_tokens"
)

// Limits 是套餐的配额上限；0 表示不限。
type Limits struct {
	MaxUsers    int      `json:"max_users"`
	StorageGB   int      `json:"storage_gb"`
	APIQPS      int      `json:"api_qps"`
	MCPCalls    int64    `json:"mcp_calls"`
	MCPTokens   int64    `json:"mcp_tokens"`
	MCPSessions int      `json:"mcp_sessions"`
	Features    []string `json:"features"`
}

// ParseLimits 解析 plans.quota 的 JSON。
func ParseLimits(raw string) (Limits, error) {
	if raw == "" {
		return Limits{}, nil
	}

	var limits Limits
	if err := json.Unmarshal([]byte(raw), &limits); err != nil {
		return Limits{}, fmt.Errorf("quota: parse plan limits: %w", err)
	}

	return limits, nil
}

// ForMetric 返回指标对应的上限。
func (l Limits) ForMetric(metric Metric) int64 {
	switch metric {
	case MetricAPICalls:
		return int64(l.APIQPS) * 1000 // 粗略上限，精确限流由限流中间件负责
	case MetricMCPCalls:
		return l.MCPCalls
	case MetricMCPToken:
		return l.MCPTokens
	default:
		return 0
	}
}

// HasFeature 判断套餐是否包含某功能。
func (l Limits) HasFeature(name string) bool {
	for _, feature := range l.Features {
		if feature == name {
			return true
		}
	}
	return false
}
