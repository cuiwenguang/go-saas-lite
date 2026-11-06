package stats

// 模块内原生 SQL 集中在此（readme 3.2.1）；平台组表不走租户隔离，聚合查询用 Raw 表达。

// overviewSQL 一次取回总览需要的全部计数。
const overviewSQL = `
SELECT
    COUNT(*)                                                                     AS tenant_total,
    COUNT(*) FILTER (WHERE status = 'active')                                    AS tenant_active,
    COUNT(*) FILTER (WHERE created_at >= date_trunc('week', now()))              AS new_this_week,
    COUNT(*) FILTER (WHERE status = 'active'
                       AND expire_at IS NOT NULL
                       AND expire_at <= now() + interval '30 days')              AS expiring_soon,
    COUNT(*) FILTER (WHERE provision_status = 'failed')                          AS provision_failed
FROM tenants`

// mcpCallsTotalSQL 汇总全部租户的 MCP 调用量（worker 已落到 usage_daily）。
const mcpCallsTotalSQL = `
SELECT COALESCE(SUM(value), 0) AS total
FROM usage_daily
WHERE metric = 'mcp_calls'`

// newTenantsSeriesSQL 近 7 天每日新增租户，缺失的日期补 0。
const newTenantsSeriesSQL = `
SELECT to_char(day, 'MM-DD') AS date, COALESCE(t.total, 0) AS value
FROM generate_series(current_date - interval '6 days', current_date, interval '1 day') AS day
LEFT JOIN (
    SELECT date_trunc('day', created_at) AS created_day, COUNT(*) AS total
    FROM tenants
    GROUP BY 1
) t ON t.created_day = day
ORDER BY day`

// usageTopSQL 按指标取用量最高的租户。
const usageTopSQL = `
SELECT u.tenant_id, t.name, SUM(u.value) AS value
FROM usage_daily u
JOIN tenants t ON t.id = u.tenant_id
WHERE u.metric = ?
GROUP BY u.tenant_id, t.name
ORDER BY value DESC
LIMIT ?`
