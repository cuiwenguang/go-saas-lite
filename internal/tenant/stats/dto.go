package stats

// MemberStats 是租户成员统计。
type MemberStats struct {
	Total  int64 `json:"total"`
	Active int64 `json:"active"`
}

// PlanStats 是当前套餐与限额（限额 0 表示不限）。
type PlanStats struct {
	Code     string         `json:"code"`
	Name     string         `json:"name"`
	MaxUsers int64          `json:"max_users"`
	Quota    map[string]any `json:"quota"`
}

// UsageStats 是近 30 天用量。
type UsageStats struct {
	Calls30d      int64            `json:"calls_30d"`
	Failed30d     int64            `json:"failed_30d"`
	AvgDurationMS int64            `json:"avg_duration_ms_30d"`
	ActiveAPIKeys int64            `json:"active_api_keys"`
	MetricTotals  map[string]int64 `json:"metric_totals"`
}

// SeriesPoint 是趋势图上的一个点。
type SeriesPoint struct {
	Date  string `json:"date"`
	Value int64  `json:"value"`
}

// OverviewOutput 是租户总览出参。
type OverviewOutput struct {
	Members    MemberStats   `json:"members"`
	Plan       PlanStats     `json:"plan"`
	Usage      UsageStats    `json:"usage"`
	CallSeries []SeriesPoint `json:"call_series"`
}
