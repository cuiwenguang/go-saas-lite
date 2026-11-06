package plans

// 模块内原生 SQL 集中在此（readme 3.2.1）；平台组表不走租户隔离。

// tenantCountsSQL 统计每个套餐被多少租户引用。
const tenantCountsSQL = `
SELECT plan_code, COUNT(*) AS total
FROM tenants
GROUP BY plan_code`

// tenantCountByCodeSQL 统计单个套餐的引用数。
const tenantCountByCodeSQL = `
SELECT COUNT(*) AS total
FROM tenants
WHERE plan_code = ?`
