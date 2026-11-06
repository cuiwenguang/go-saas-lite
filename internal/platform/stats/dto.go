package stats

// SeriesPoint 是趋势图上的一个点。
type SeriesPoint struct {
	Date  string `json:"date"`
	Value int64  `json:"value"`
}

// OverviewOutput 是总览页需要的全部数字。
type OverviewOutput struct {
	TenantTotal     int64         `json:"tenant_total"`
	TenantActive    int64         `json:"tenant_active"`
	NewThisWeek     int64         `json:"new_this_week"`
	ExpiringSoon    int64         `json:"expiring_soon"`
	ProvisionFailed int64         `json:"provision_failed"`
	MCPCallsTotal   int64         `json:"mcp_calls_total"`
	NewSeries       []SeriesPoint `json:"new_series"`
}

// UsageTopOutput 是用量排行的一行。
type UsageTopOutput struct {
	TenantID string `json:"tenant_id"`
	Name     string `json:"name"`
	Value    int64  `json:"value"`
}
