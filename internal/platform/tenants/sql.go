package tenants

// 模块内原生 SQL 集中在此，便于评审与后续换库改写（readme 3.2.1）。
// 平台组表不走租户隔离，无需 tenant_id 条件。

// summarySQL 按状态聚合租户数量（ORM 链式 API 没有 GROUP BY，用 Raw 表达）。
const summarySQL = `
SELECT status, COUNT(*) AS total
FROM tenants
GROUP BY status`

// tenantUsageSQL 取租户近 30 天各指标累计用量（usage_daily 由 worker 落库）。
const tenantUsageSQL = `
SELECT metric, COALESCE(SUM(value), 0) AS total
FROM usage_daily
WHERE tenant_id = ?
  AND day >= current_date - interval '29 days'
GROUP BY metric`

// tenantCallSeriesSQL 取租户近 7 天 MCP 调用趋势，缺失日期补 0。
const tenantCallSeriesSQL = `
SELECT to_char(d.day, 'MM-DD') AS date, COALESCE(u.total, 0) AS value
FROM generate_series(current_date - interval '6 days', current_date, interval '1 day') AS d(day)
LEFT JOIN (
    SELECT day AS usage_day, SUM(value) AS total
    FROM usage_daily
    WHERE tenant_id = ?
      AND metric = 'mcp_calls'
      AND day >= current_date - interval '6 days'
    GROUP BY day
) u ON u.usage_day = d.day
ORDER BY d.day`
