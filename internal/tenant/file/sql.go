package file

// storageUsageSQL 统计租户已用存储字节数。
const storageUsageSQL = `
SELECT COALESCE(SUM(size_bytes), 0) AS used
FROM tenant_files
WHERE tenant_id = ?`
