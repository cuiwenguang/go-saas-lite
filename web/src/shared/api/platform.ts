import { del, get, patch, post } from "@/shared/http/client";
import type {
  AuditLogRow,
  CreateTenantInput,
  CredentialResult,
  Paged,
  Plan,
  PlatformStats,
  Quota,
  TenantDetail,
  TenantRow,
  TenantStatus,
  TenantSummary,
  TenantUsage,
  TenantUser,
  UsageTopRow,
} from "./types";

/* 平台侧：租户管理与套餐。URL 唯一出处（CONVENTIONS-FRONTEND §4） */

export type { TenantSummary };

export interface TenantListParams {
  page?: number
  size?: number
  status?: string
  keyword?: string
  plan_code?: string
  /** RFC3339：筛选该时间点前到期的租户 */
  expire_before?: string
}

export function listTenants(params: TenantListParams) {
  return get<Paged<TenantRow>>("/api/platform/v1/tenants", params as Record<string, unknown>);
}

/** 状态分布计数（分段筛选用） */
export function tenantSummary() {
  return get<TenantSummary>("/api/platform/v1/tenants/summary");
}

export function getTenant(id: string) {
  return get<TenantDetail>(`/api/platform/v1/tenants/${id}`);
}

export function createTenant(body: CreateTenantInput) {
  return post<CredentialResult>("/api/platform/v1/tenants", body);
}

/** 改基本信息 / 换套餐 / 续费（只传要改的字段） */
export interface UpdateTenantBody {
  name?: string
  plan_code?: string
  expire_at?: string
  timezone?: string
  locale?: string
  contact_name?: string
  contact_phone?: string
  contact_email?: string
}

export function updateTenant(id: string, body: UpdateTenantBody) {
  return patch<TenantRow>(`/api/platform/v1/tenants/${id}`, body);
}

export function setTenantStatus(id: string, status: TenantStatus) {
  return patch<TenantRow>(`/api/platform/v1/tenants/${id}/status`, { status });
}

/** readme 契约：删除 = 转 archived */
export function archiveTenant(id: string) {
  return del<{ id: string; status: string }>(`/api/platform/v1/tenants/${id}`);
}

/** 断点重跑开通流水线，返回新的管理员初始密码 */
export function retryProvision(id: string) {
  return post<CredentialResult>(`/api/platform/v1/tenants/${id}/retry`);
}

/** 重置租户管理员密码，返回一次性密码 */
export function resetTenantAdminPassword(id: string) {
  return post<CredentialResult>(`/api/platform/v1/tenants/${id}/admin-password`);
}

export function listTenantUsers(id: string) {
  return get<{ items: TenantUser[] }>(`/api/platform/v1/tenants/${id}/users`);
}

/** 配额与用量：套餐限额 + 近 30 天用量 + 近 7 天趋势 */
export function getTenantUsage(id: string) {
  return get<TenantUsage>(`/api/platform/v1/tenants/${id}/usage`);
}

export function listPlans() {
  return get<Plan[]>("/api/platform/v1/plans");
}

export function createPlan(body: { code: string; name: string; quota: Quota; enabled?: boolean }) {
  return post<Plan>("/api/platform/v1/plans", body);
}

/** code 创建后不可改；只传要改的字段 */
export function updatePlan(id: string, body: { name?: string; quota?: Partial<Plan["quota"]>; enabled?: boolean }) {
  return patch<Plan>(`/api/platform/v1/plans/${id}`, body);
}

export function platformStats() {
  return get<PlatformStats>("/api/platform/v1/stats/overview");
}

/** 用量排行（默认 MCP 调用 Top5） */
export function usageTop(params?: { metric?: string; limit?: number }) {
  return get<UsageTopRow[]>("/api/platform/v1/stats/usage-top", params as Record<string, unknown>);
}

/* ---------- 审计日志 ---------- */

export interface AuditListParams {
  page?: number
  size?: number
  category?: string
  action?: string
  level?: string
  actor?: string
  /** RFC3339 */
  from?: string
  to?: string
}

export function listAuditLogs(params: AuditListParams) {
  return get<Paged<AuditLogRow>>("/api/platform/v1/audit-logs", params as Record<string, unknown>);
}

