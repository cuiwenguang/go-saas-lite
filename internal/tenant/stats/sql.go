package stats

// 模块内原生 SQL 集中在此（readme 3.2.1）。租户连接上的 Raw 不经过隔离 Callback，
// 因此每条语句都显式写出 tenant_id 条件，并只用参数化单条 SELECT。

// callAggSQL 汇总近 30 天调用次数、失败次数与平均耗时。
const callAggSQL = `
SELECT COUNT(*)                                                     AS total,
       COUNT(*) FILTER (WHERE status <> 'ok')                       AS failed,
       COALESCE(AVG(duration_ms), 0)::bigint                        AS avg_milli
FROM mcp_call_log
WHERE tenant_id = ? AND created_at >= ?`

// callSeriesSQL 按天分组近 7 天调用量，缺失日期由应用层补 0。
const callSeriesSQL = `
SELECT date_trunc('day', created_at) AS day, COUNT(*) AS total
FROM mcp_call_log
WHERE tenant_id = ? AND created_at >= ?
GROUP BY 1`

// activeKeysSQL 统计当前租户未撤销的 API Key。
const activeKeysSQL = `
SELECT COUNT(*) AS total
FROM api_keys
WHERE tenant_id = ? AND revoked_at IS NULL`

// usageTotalsSQL 读取近 30 天用量快照（worker 落库的 usage_daily）。
const usageTotalsSQL = `
SELECT metric, SUM(value) AS total
FROM usage_daily
WHERE tenant_id = ? AND day >= ?
GROUP BY metric`
