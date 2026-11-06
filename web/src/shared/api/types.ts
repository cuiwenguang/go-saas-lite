/* 统一 DTO 类型：字段命名与后端 readme 契约对齐（snake_case）。
 * 手写过渡期产物，后续由 api/openapi.yaml 生成替换（CONVENTIONS-FRONTEND §4） */

/* ---------- 通用 ---------- */

export interface Paged<T> {
  items: T[]
  total: number
  page: number
  size: number
}

export interface SeriesPoint {
  date: string
  value: number
}

export interface Quota {
  max_users: number
  storage_gb: number
  api_qps: number
  mcp_calls: number
  mcp_tokens: number
  mcp_sessions: number
  features: string[]
}

export interface QuotaUsage {
  users: number
  storage_gb: number
  api_calls: number
  mcp_calls: number
  mcp_tokens: number
  mcp_sessions: number
}

export type TenantStatus = 'pending' | 'active' | 'suspended' | 'expired' | 'archived'

export type ProvisionStatus = 'pending' | 'running' | 'succeeded' | 'failed'

/* ---------- 平台侧 ---------- */

export interface PlatformMe {
  user: { id: string; email: string; role: PlatformRole }
  role: PlatformRole
  permissions: string[]
}

export type PlatformRole = 'owner' | 'viewer'

/** 登录 / 刷新返回的会话（refresh 令牌在 httpOnly cookie，前端不接触） */
export interface PlatformSession {
  access_token: string
  token_type: string
  expires_in: number
  role: PlatformRole
}

/* 租户出参 = 后端 TenantOutput；字段为空的用可选表示 */
export interface TenantRow {
  id: string
  code: string
  name: string
  domain?: string
  subdomain?: string
  status: TenantStatus
  plan_code: string
  /** 套餐名称（套餐接口接入前用 code 兑底展示） */
  plan_name?: string
  expire_at?: string
  timezone: string
  locale: string
  contact_name?: string
  contact_phone?: string
  contact_email?: string
  admin_email?: string
  provision_status: ProvisionStatus
  provision_step?: string
  provision_log: ProvisionEvent[]
  created_at: string
  updated_at: string
}

/** 详情与列表同构：详情就是单条 TenantOutput */
export type TenantDetail = TenantRow

export interface ProvisionEvent {
  step: string
  ok: boolean
  error?: string
  at: string
}

/** 列表分段筛选与总览用的状态计数 */
export interface TenantSummary {
  total: number
  pending: number
  active: number
  suspended: number
  expired: number
  archived: number
}

export interface PlatformStats {
  tenant_total: number
  tenant_active: number
  new_this_week: number
  /** 30 天内到期的活跃租户 */
  expiring_soon: number
  /** 开通失败的租户数（待处理） */
  provision_failed: number
  mcp_calls_total: number
  new_series: SeriesPoint[]
}

/** 用量排行的一行 */
export interface UsageTopRow {
  tenant_id: string
  name: string
  value: number
}

export interface Plan {
  id: string
  code: string
  name: string
  quota: Quota
  enabled: boolean
  /** 引用该套餐的租户数 */
  tenant_count: number
  created_at: string
  updated_at: string
}

/** 开通租户入参，字段与后端 CreateInput 对齐 */
export interface CreateTenantInput {
  code: string
  name: string
  timezone: string
  locale: string
  plan_code: string
  subdomain?: string
  domain?: string
  contact_name?: string
  contact_phone?: string
  contact_email?: string
  admin_email: string
  admin_display_name?: string
  admin_password?: string
}

/** 创建 / 重试 / 重置密码的返回：初始密码只出现一次 */
export interface CredentialResult {
  tenant: TenantRow
  initial_password: string
}

/** 租户内用户（详情页只读展示） */
export interface TenantUser {
  id: string
  email: string
  display_name: string
  status: string
  roles: string[]
  must_change_password: boolean
  created_at: string
}

/** 租户详情「配额与用量」：限额来自当前套餐，用量为近 30 天累计 */
export interface TenantUsage {
  plan_code: string
  max_users: number
  mcp_calls_cap: number
  mcp_tokens_cap: number
  api_qps: number
  usage: {
    users: number
    mcp_calls: number
    api_calls: number
    mcp_tokens: number
  }
  series: SeriesPoint[]
}

/* ---------- 审计日志 ---------- */

export type AuditLevel = 'info' | 'warn'
export type AuditResult = 'success' | 'failed'

export interface AuditLogRow {
  id: string
  category: string
  action: string
  level: AuditLevel
  result: AuditResult
  actor_type: 'admin' | 'system'
  actor_email?: string
  object_type: string
  object_id: string
  summary: string
  /** 变更前后的字段快照，详情抽屉做 diff */
  before?: Record<string, unknown>
  after?: Record<string, unknown>
  request_id?: string
  ip?: string
  created_at: string
}

/* ---------- 租户侧 ---------- */

export interface TenantMe {
  user: { id: string; email: string; display_name: string; roles: string[] }
  tenant: { id: string; name: string; subdomain: string; plan_id: string; plan_name: string }
  permissions: string[]
}

export interface UserRow {
  id: string
  email: string
  display_name: string
  status: 'active' | 'disabled'
  roles: string[]
  created_at: string
  last_login_at: string
}

export interface InviteUserResult {
  user: UserRow
  initial_password: string
}

export interface ApiKeyRow {
  id: string
  name: string
  prefix: string
  owner_name: string
  status: 'active' | 'revoked'
  created_at: string
  last_used_at: string
}

export interface CreateApiKeyResult {
  key: ApiKeyRow
  /** 一次性明文 secret，仅创建响应返回一次 */
  secret: string
}

export interface McpServerRow {
  id: string
  name: string
  url: string
  status: 'connected' | 'error'
  tool_count: number
  description: string
}

export interface McpToolRow {
  id: string
  server_id: string
  server_name: string
  name: string
  description: string
  required_perms: string[]
  enabled: boolean
  calls_30d: number
}

export interface TenantStats {
  usage: QuotaUsage
  quota: Quota
  call_series: SeriesPoint[]
}
