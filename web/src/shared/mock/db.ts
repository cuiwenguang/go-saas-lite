import type {
  ApiKeyRow, AuditLogRow, McpServerRow, McpToolRow, Plan, Quota, SeriesPoint, TenantRow, UserRow,
} from "../api/types";

/* mock 内存数据库：仅原型演示用。真实后端就绪后删除整个 shared/mock 目录。 */

export interface MockState {
  platformToken: string
  tenantToken: string
  tenants: TenantRow[]
  plans: Plan[]
  users: UserRow[]
  apiKeys: ApiKeyRow[]
  mcpServers: McpServerRow[]
  mcpTools: McpToolRow[]
  auditLogs: AuditLogRow[]
  tenantUsage: { users: number; storage_gb: number; api_calls: number; mcp_calls: number; mcp_tokens: number; mcp_sessions: number }
}

export const freeQuota: Quota = { max_users: 3, storage_gb: 2, api_qps: 10, mcp_calls: 1_000, mcp_tokens: 10_000, mcp_sessions: 5, features: [] }
export const proQuota: Quota = { max_users: 10, storage_gb: 50, api_qps: 50, mcp_calls: 100_000, mcp_tokens: 5_000_000, mcp_sessions: 50, features: ["sso"] }
export const entQuota: Quota = { max_users: 0, storage_gb: 500, api_qps: 200, mcp_calls: 1_000_000, mcp_tokens: 50_000_000, mcp_sessions: 200, features: ["sso", "audit", "dedicated"] }

function series(base: number, wobble: number[]): SeriesPoint[] {
  const days = ["10-31", "11-01", "11-02", "11-03", "11-04", "11-05", "11-06"]
  return days.map((d, i) => ({ date: d, value: base + wobble[i % wobble.length] * (i + 1) }))
}

const planNameByCode: Record<string, string> = { free: "Free", pro: "Pro", enterprise: "Enterprise" }

/** 审计种子：时间相对当前，便于演示「近 7 天」筛选 */
const auditSeed = (minutesAgo: number, seed: Partial<AuditLogRow> & Pick<AuditLogRow, "category" | "action" | "summary">): AuditLogRow => ({
  id: randId("log"),
  level: "info",
  result: "success",
  actor_type: "admin",
  actor_email: "admin@example.com",
  object_type: "",
  object_id: "",
  ip: "10.0.4.21",
  created_at: new Date(Date.now() - minutesAgo * 60_000).toISOString(),
  ...seed,
})

const seedPlan = (id: string, code: string, name: string, quota: Plan["quota"]): Plan => ({
  id, code, name, quota, enabled: true, tenant_count: 0,
  created_at: "2026-05-01T00:00:00Z", updated_at: "2026-05-01T00:00:00Z",
})

type TenantSeed = Pick<TenantRow, "id" | "code" | "name" | "status" | "plan_code" | "created_at"> &
  Partial<Omit<TenantRow, "id" | "code" | "name" | "status" | "plan_code" | "created_at">>

/** mock 租户工厂：按后端 TenantOutput 补齐默认字段，避免每条数据重复写 */
export function mkTenant(seed: TenantSeed): TenantRow {
  return {
    id: seed.id,
    code: seed.code,
    name: seed.name,
    domain: seed.domain,
    subdomain: seed.subdomain,
    status: seed.status,
    plan_code: seed.plan_code,
    plan_name: planNameByCode[seed.plan_code] ?? seed.plan_code,
    expire_at: seed.expire_at,
    timezone: seed.timezone ?? "Asia/Shanghai",
    locale: seed.locale ?? "zh-CN",
    contact_name: seed.contact_name,
    contact_phone: seed.contact_phone,
    contact_email: seed.contact_email,
    admin_email: seed.admin_email,
    provision_status: seed.provision_status ?? "succeeded",
    provision_step: seed.provision_step ?? "done",
    provision_log: seed.provision_log ?? [
      { step: "tenant_created", ok: true, at: seed.created_at },
      { step: "tenant_bootstrapped", ok: true, at: seed.created_at },
      { step: "done", ok: true, at: seed.created_at },
    ],
    created_at: seed.created_at,
    updated_at: seed.updated_at ?? seed.created_at,
  }
}

export const db: MockState = {
  platformToken: "",
  tenantToken: "",

  plans: [
    seedPlan("plan_free", "free", "Free", freeQuota),
    seedPlan("plan_pro", "pro", "Pro", proQuota),
    seedPlan("plan_ent", "enterprise", "Enterprise", entQuota),
  ],

  tenants: [mkTenant({ id: "t_9f2a", code: "acme", name: "Acme 科技", subdomain: "acme", plan_code: "pro", status: "active", admin_email: "owner@acme.com", expire_at: "2027-08-12T00:00:00Z", created_at: "2026-08-12T09:30:00Z" }),
  mkTenant({ id: "t_7c1d", code: "initech", name: "Initech", subdomain: "initech", plan_code: "enterprise", status: "active", admin_email: "ops@initech.com", expire_at: "2027-06-03T00:00:00Z", created_at: "2026-06-03T14:00:00Z" }),
  mkTenant({ id: "t_5b8e", code: "globex", name: "Globex", subdomain: "globex", plan_code: "pro", status: "active", admin_email: "it@globex.com", expire_at: "2027-09-21T00:00:00Z", created_at: "2026-09-21T11:00:00Z" }),
  mkTenant({
    id: "t_3a4f", code: "umbrella", name: "Umbrella", subdomain: "umbrella", plan_code: "free", status: "pending",
    admin_email: "admin@umbrella.com", created_at: "2026-10-02T08:00:00Z",
    provision_status: "failed", provision_step: "failed",
    provision_log: [
      { step: "tenant_created", ok: true, at: "2026-10-02T08:00:05Z" },
      { step: "failed", ok: false, error: "ensure admin user: email already exists", at: "2026-10-02T08:00:09Z" },
    ],
  }),
  mkTenant({ id: "t_8d2c", code: "stark", name: "Stark 工业", subdomain: "stark", plan_code: "pro", status: "active", admin_email: "it@stark.com", expire_at: "2027-10-28T00:00:00Z", created_at: "2026-10-28T16:30:00Z" }),
  mkTenant({ id: "t_6e9b", code: "wayne", name: "Wayne 企业", subdomain: "wayne", plan_code: "free", status: "active", admin_email: "it@wayne.com", expire_at: "2026-11-20T00:00:00Z", created_at: "2026-11-01T10:00:00Z" }),
  mkTenant({ id: "t_1f7a", code: "cyberdyne", name: "Cyberdyne", subdomain: "cyberdyne", plan_code: "pro", status: "active", admin_email: "it@cyberdyne.com", expire_at: "2026-12-03T00:00:00Z", created_at: "2026-11-03T09:00:00Z" }),
  mkTenant({ id: "t_4c3e", code: "soylent", name: "Soylent", subdomain: "soylent", plan_code: "free", status: "archived", admin_email: "it@soylent.com", created_at: "2026-05-19T12:00:00Z" }),
  ],

  users: [
    { id: "u_01", email: "owner@acme.com", display_name: "张伟", status: "active", roles: ["admin"], created_at: "2026-08-12T09:35:00Z", last_login_at: "2026-11-06T08:12:00Z" },
    { id: "u_02", email: "li.dev@acme.com", display_name: "李工", status: "active", roles: ["member"], created_at: "2026-08-20T10:00:00Z", last_login_at: "2026-11-05T19:40:00Z" },
    { id: "u_03", email: "wang.ops@acme.com", display_name: "王运维", status: "active", roles: ["member"], created_at: "2026-09-01T09:00:00Z", last_login_at: "2026-11-06T07:55:00Z" },
    { id: "u_04", email: "zhao.dev@acme.com", display_name: "赵敏", status: "active", roles: ["member"], created_at: "2026-09-15T14:00:00Z", last_login_at: "2026-11-04T16:20:00Z" },
    { id: "u_05", email: "sun.pm@acme.com", display_name: "孙倩", status: "active", roles: ["member"], created_at: "2026-09-28T11:30:00Z", last_login_at: "2026-11-03T10:10:00Z" },
    { id: "u_06", email: "zhou.qa@acme.com", display_name: "周磊", status: "disabled", roles: ["member"], created_at: "2026-10-05T09:00:00Z", last_login_at: "2026-10-20T09:00:00Z" },
    { id: "u_07", email: "wu.dev@acme.com", display_name: "吴静", status: "active", roles: ["member"], created_at: "2026-10-30T15:00:00Z", last_login_at: "" },
    { id: "u_08", email: "liu.fin@acme.com", display_name: "刘会计", status: "active", roles: ["member"], created_at: "2026-11-02T10:00:00Z", last_login_at: "" },
  ],

  apiKeys: [
    { id: "k_01", name: "CI 集成", prefix: "sk_live_9fA2", owner_name: "张伟", status: "active", created_at: "2026-09-10T10:00:00Z", last_used_at: "2026-11-06T07:58:00Z" },
    { id: "k_02", name: "数据同步脚本", prefix: "sk_live_c7Bd", owner_name: "李工", status: "active", created_at: "2026-10-12T14:30:00Z", last_used_at: "2026-11-05T22:01:00Z" },
    { id: "k_03", name: "旧版报表导出", prefix: "sk_live_44Ee", owner_name: "张伟", status: "revoked", created_at: "2026-08-25T09:00:00Z", last_used_at: "2026-10-01T06:30:00Z" },
  ],

  mcpServers: [
    { id: "s_wx", name: "weather-api", url: "https://mcp.weather.example.com/sse", status: "connected", tool_count: 3, description: "实时天气与空气质量查询" },
    { id: "s_gh", name: "github", url: "https://mcp.github.com/streamable", status: "connected", tool_count: 3, description: "仓库、Issue、PR 检索" },
    { id: "s_doc", name: "docs-search", url: "https://internal.acme.com/mcp", status: "error", tool_count: 2, description: "内部文档知识库检索（内网）" },
  ],

  mcpTools: [
    { id: "m_01", server_id: "s_wx", server_name: "weather-api", name: "get_current_weather", description: "查询指定城市当前天气", required_perms: ["weather:read"], enabled: true, calls_30d: 3_120 },
    { id: "m_02", server_id: "s_wx", server_name: "weather-api", name: "get_air_quality", description: "查询城市空气质量指数", required_perms: ["weather:read"], enabled: true, calls_30d: 892 },
    { id: "m_03", server_id: "s_wx", server_name: "weather-api", name: "get_forecast", description: "未来 7 天天气预报", required_perms: ["weather:read"], enabled: false, calls_30d: 0 },
    { id: "m_04", server_id: "s_gh", server_name: "github", name: "search_repositories", description: "按关键词搜索仓库", required_perms: ["repo:read"], enabled: true, calls_30d: 1_740 },
    { id: "m_05", server_id: "s_gh", server_name: "github", name: "list_issues", description: "列出仓库 Issue", required_perms: ["repo:read"], enabled: true, calls_30d: 980 },
    { id: "m_06", server_id: "s_gh", server_name: "github", name: "create_issue", description: "在仓库创建 Issue", required_perms: ["repo:create"], enabled: true, calls_30d: 61 },
    { id: "m_07", server_id: "s_doc", server_name: "docs-search", name: "search_docs", description: "全文检索内部文档", required_perms: ["doc:read"], enabled: true, calls_30d: 4_502 },
    { id: "m_08", server_id: "s_doc", server_name: "docs-search", name: "summarize_doc", description: "对文档生成摘要", required_perms: ["doc:read"], enabled: false, calls_30d: 0 },
  ],

  auditLogs: [
    auditSeed(12, { category: "auth", action: "platform.login", summary: "平台管理员登录" }),
    auditSeed(35, { category: "tenant", action: "tenant.create", object_type: "tenant", object_id: "t_1f7a", summary: "开通租户 Cyberdyne", after: { name: "Cyberdyne", plan_code: "pro" } }),
    auditSeed(48, { category: "tenant", action: "tenant.renew", object_type: "tenant", object_id: "t_6e9b", summary: "续费租户 Wayne 企业", before: { expire_at: "2026-11-01T00:00:00Z" }, after: { expire_at: "2026-11-20T00:00:00Z" } }),
    auditSeed(90, { category: "plan", action: "plan.update", object_type: "plan", object_id: "plan_pro", summary: "修改套餐 Pro", before: { quota: { max_users: 8 } }, after: { quota: { max_users: 10 } } }),
    auditSeed(150, { category: "security", action: "tenant.cross_tenant_read", level: "warn", object_type: "tenant", object_id: "t_9f2a", summary: "跨租户查看租户用户（8 人）" }),
    auditSeed(180, { category: "tenant", action: "tenant.reset_admin_password", level: "warn", object_type: "tenant", object_id: "t_3a4f", summary: "重置租户管理员密码 Umbrella", after: { admin_email: "admin@umbrella.com" } }),
    auditSeed(240, { category: "auth", action: "platform.login_failed", level: "warn", result: "failed", actor_email: "unknown@example.com", summary: "平台登录失败：unknown@example.com" }),
    auditSeed(320, { category: "system", action: "tenant.auto_expire", actor_type: "system", actor_email: undefined, object_type: "tenant", object_id: "t_4c3e", summary: "自动流转到期租户 Soylent" }),
    auditSeed(400, { category: "tenant", action: "tenant.update_status", level: "warn", object_type: "tenant", object_id: "t_4c3e", summary: "变更租户状态 Soylent → archived", before: { status: "expired" }, after: { status: "archived" } }),
    auditSeed(600, { category: "plan", action: "plan.create", object_type: "plan", object_id: "plan_ent", summary: "新建套餐 Enterprise", after: { code: "enterprise", name: "Enterprise" } }),
    auditSeed(700, { category: "auth", action: "platform.password_change", level: "warn", summary: "修改平台账号密码" }),
    auditSeed(900, { category: "config", action: "config.update", summary: "调整平台配置：审计日志保留天数 → 90" }),
  ],

  tenantUsage: { users: 8, storage_gb: 31.4, api_calls: 18_920, mcp_calls: 23_410, mcp_tokens: 1_802_000, mcp_sessions: 12 },
}

export const adminPermissions = [
  "tenant:read", "tenant:create", "tenant:update", "tenant:delete",
  "plan:read", "plan:update", "stats:read", "audit:read",
]

export const tenantPermissions = [
  "user:read", "user:create", "user:update", "user:delete",
  "apikey:read", "apikey:create", "apikey:revoke",
  "mcp:read", "mcp:update",
  "stats:read",
]

export const tenantCallSeries = series(2_800, [180, -120, 240, 60, -200, 320])
export const newTenantSeries = series(2, [1, 0, 2, 1, 0, 1])

export function randId(prefix: string): string {
  return `${prefix}_${Math.random().toString(36).slice(2, 6)}`
}

export function randomPassword(): string {
  const chars = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz23456789"
  return Array.from({ length: 10 }, () => chars[Math.floor(Math.random() * chars.length)]).join("")
}

export function randomSecret(): string {
  return `sk_live_${Math.random().toString(36).slice(2, 10)}${Math.random().toString(36).slice(2, 10)}`
}
