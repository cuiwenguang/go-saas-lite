/* 当前租户上下文
 *
 * 登录是全局的（邮箱全局唯一），登录请求不需要任何租户信息；
 * 登录后由 `GET /api/tenant/v1/me` 返回所属租户，之后每个请求用 X-Tenant-Id 带上它。
 * 服务端以令牌里的 tenant_id 为准，这个头只作为「当前选择」的显式声明，
 * 为将来一人多租户（切换租户）留扩展位。
 */

const STORAGE_KEY = "saas.tenant.id";

let current = localStorage.getItem(STORAGE_KEY) || "";

/** 当前租户 ID（未登录时为空） */
export function currentTenantKey(): string {
  return current;
}

/** 登录 / 刷新资料后由 store 调用 */
export function setCurrentTenant(tenantID: string) {
  current = tenantID.trim();
  if (current) localStorage.setItem(STORAGE_KEY, current);
  else localStorage.removeItem(STORAGE_KEY);
}

/** 登出时清掉，避免下一个账号沿用 */
export function clearTenant() {
  current = "";
  localStorage.removeItem(STORAGE_KEY);
}
