import type { Paged } from "../api/types";
import {
  adminPermissions, db, mkTenant, newTenantSeries, proQuota, randId, randomPassword, randomSecret,
  tenantCallSeries, tenantPermissions,
} from "./db";

/* mock 路由表：URL / 字段与后端 readme 契约一致，真实后端就绪后整目录删除 */

export interface MockCtx {
  params: Record<string, string>
  query: Record<string, string>
  body: Record<string, unknown>
  headers: Record<string, unknown>
}

export interface MockRoute {
  method: "get" | "post" | "patch" | "delete"
  pattern: string
  handler: (ctx: MockCtx) => unknown
}

const err = (code: number, msg: string) => ({ code, msg })

function bearer(ctx: MockCtx): string {
  const h = ctx.headers as Record<string, unknown> & { get?: (k: string) => unknown }
  /* axios 1.x 传入的是 AxiosHeaders 实例：属性名大小写不定，优先用 get() */
  const raw = (h.get?.("Authorization") ?? h.Authorization ?? h.authorization ?? "") as string
  return raw.replace(/^Bearer\s+/i, "")
}

function requirePlatform(ctx: MockCtx) {
  return bearer(ctx) === "mock-platform-token" ? null : err(401, "登录已失效")
}
function requireTenant(ctx: MockCtx) {
  return bearer(ctx) === "mock-tenant-token" ? null : err(401, "登录已失效")
}

function paged<T>(items: T[], query: Record<string, string>): Paged<T> {
  const page = Math.max(1, parseInt(query.page || "1", 10) || 1)
  const pageSize = Math.min(50, Math.max(1, parseInt(query.size || query.page_size || "10", 10) || 10))
  return { items: items.slice((page - 1) * pageSize, page * pageSize), total: items.length, page, size: pageSize }
}

export const routes: MockRoute[] = [
  /* ================= 平台侧 ================= */

  {
    method: "post",
    pattern: "/api/platform/v1/auth/login",
    handler: ({ body }) => {
      const email = String(body.email || "")
      const password = String(body.password || "")
      if (!email.includes("@") || password.length < 6) return err(401, "邮箱或密码错误")
      db.platformToken = "mock-platform-token"
      return { access_token: "mock-platform-token", token_type: "Bearer", expires_in: 900, role: "owner" }
    },
  },
  {
    /* mock 不模拟 cookie 轮换：直接发新令牌，真实后端由 httpOnly cookie 承托 */
    method: "post",
    pattern: "/api/platform/v1/auth/refresh",
    handler: () => ({ access_token: "mock-platform-token", token_type: "Bearer", expires_in: 900, role: "owner" }),
  },
  {
    method: "post",
    pattern: "/api/platform/v1/auth/logout",
    handler: () => ({ ok: true }),
  },
  {
    method: "get",
    pattern: "/api/platform/v1/me",
    handler: (ctx) => {
      const e = requirePlatform(ctx); if (e) return e
      return {
        user: { id: "pu_01", email: "admin@example.com", role: "owner" },
        role: "owner",
        permissions: adminPermissions,
      }
    },
  },
  {
    method: "get",
    pattern: "/api/platform/v1/stats/overview",
    handler: (ctx) => {
      const e = requirePlatform(ctx); if (e) return e
      const soon = Date.now() + 30 * 86_400_000
      return {
        tenant_total: db.tenants.length,
        tenant_active: db.tenants.filter((t) => t.status === "active").length,
        new_this_week: db.tenants.filter((t) => t.created_at > "2026-11-01").length,
        expiring_soon: db.tenants.filter((t) =>
          t.status === "active" && t.expire_at && new Date(t.expire_at).getTime() <= soon).length,
        provision_failed: db.tenants.filter((t) => t.provision_status === "failed").length,
        mcp_calls_total: 265_641,
        new_series: newTenantSeries,
      }
    },
  },
  {
    method: "get",
    pattern: "/api/platform/v1/stats/usage-top",
    handler: (ctx) => {
      const e = requirePlatform(ctx); if (e) return e
      const limit = Math.min(20, Math.max(1, parseInt(ctx.query.limit || "5", 10) || 5))
      return db.tenants
        .filter((t) => t.status !== "archived")
        .map((t, index) => ({ tenant_id: t.id, name: t.name, value: 182_044 - index * 33_000 }))
        .sort((a, b) => b.value - a.value)
        .slice(0, limit)
    },
  },
  {
    method: "get",
    pattern: "/api/platform/v1/tenants",
    handler: (ctx) => {
      const e = requirePlatform(ctx); if (e) return e
      let items = [...db.tenants]
      const kw = (ctx.query.keyword || "").toLowerCase()
      if (kw) {
        items = items.filter((t) =>
          t.name.toLowerCase().includes(kw) || t.code.includes(kw) || (t.subdomain || "").includes(kw))
      }
      if (ctx.query.status) items = items.filter((t) => t.status === ctx.query.status)
      if (ctx.query.plan_code) items = items.filter((t) => t.plan_code === ctx.query.plan_code)
      items.sort((a, b) => b.created_at.localeCompare(a.created_at))
      return paged(items, ctx.query)
    },
  },
  {
    /* 必须排在 /tenants/:id 之前，否则 :id 会吃掉 summary */
    method: "get",
    pattern: "/api/platform/v1/tenants/summary",
    handler: (ctx) => {
      const e = requirePlatform(ctx); if (e) return e
      const of = (s: string) => db.tenants.filter((t) => t.status === s).length
      return {
        total: db.tenants.length,
        pending: of("pending"),
        active: of("active"),
        suspended: of("suspended"),
        expired: of("expired"),
        archived: of("archived"),
      }
    },
  },
  {
    method: "get",
    pattern: "/api/platform/v1/tenants/:id/usage",
    handler: (ctx) => {
      const e = requirePlatform(ctx); if (e) return e
      const t = db.tenants.find((x) => x.id === ctx.params.id)
      if (!t) return err(40401, "租户不存在")
      const plan = db.plans.find((p) => p.code === t.plan_code)
      const quota = plan?.quota
      return {
        plan_code: t.plan_code,
        max_users: quota?.max_users ?? 0,
        mcp_calls_cap: quota?.mcp_calls ?? 0,
        mcp_tokens_cap: quota?.mcp_tokens ?? 0,
        api_qps: quota?.api_qps ?? 0,
        usage: {
          users: t.id === "t_9f2a" ? db.tenantUsage.users : 1,
          mcp_calls: t.id === "t_9f2a" ? db.tenantUsage.mcp_calls : 0,
          api_calls: t.id === "t_9f2a" ? db.tenantUsage.api_calls : 0,
          mcp_tokens: t.id === "t_9f2a" ? db.tenantUsage.mcp_tokens : 0,
        },
        series: t.id === "t_9f2a" ? tenantCallSeries : tenantCallSeries.map((p) => ({ ...p, value: 0 })),
      }
    },
  },
  {
    method: "get",
    pattern: "/api/platform/v1/tenants/:id/users",
    handler: (ctx) => {
      const e = requirePlatform(ctx); if (e) return e
      const t = db.tenants.find((x) => x.id === ctx.params.id)
      if (!t) return err(40401, "租户不存在")
      /* 演示：Acme 复用租户侧成员数据，其余只给管理员一条 */
      const items = t.id === "t_9f2a"
        ? db.users.map((u) => ({
          id: u.id, email: u.email, display_name: u.display_name, status: u.status,
          roles: u.roles, must_change_password: false, created_at: u.created_at,
        }))
        : [{
          id: "u_admin", email: t.admin_email || "-", display_name: "管理员",
          status: "active", roles: ["admin"], must_change_password: true, created_at: t.created_at,
        }]
      return { items }
    },
  },
  {
    method: "get",
    pattern: "/api/platform/v1/tenants/:id",
    handler: (ctx) => {
      const e = requirePlatform(ctx); if (e) return e
      const t = db.tenants.find((x) => x.id === ctx.params.id)
      return t ?? err(40401, "租户不存在")
    },
  },
  {
    method: "post",
    pattern: "/api/platform/v1/tenants",
    handler: (ctx) => {
      const e = requirePlatform(ctx); if (e) return e
      const name = String(ctx.body.name || "").trim()
      const code = String(ctx.body.code || "").trim().toLowerCase()
      const subdomain = String(ctx.body.subdomain || "").trim().toLowerCase()
      const planCode = String(ctx.body.plan_code || "")
      const adminEmail = String(ctx.body.admin_email || "").trim().toLowerCase()
      if (!name || !code || !adminEmail || !db.plans.some((p) => p.code === planCode)) return err(40001, "参数不完整")
      if (!/^[a-z0-9][a-z0-9-]{1,63}$/.test(code)) return err(40001, "租户标识格式不合法（小写字母或数字开头，可含 -）")
      if (db.tenants.some((t) => t.code === code)) return err(40901, "租户标识已存在")
      if (subdomain && db.tenants.some((t) => t.subdomain === subdomain)) return err(40901, "子域名已被占用")
      const now = new Date().toISOString()
      const row = mkTenant({
        id: randId("t"), code, name, status: "active", plan_code: planCode, created_at: now,
        subdomain: subdomain || undefined,
        timezone: String(ctx.body.timezone || "Asia/Shanghai"),
        locale: String(ctx.body.locale || "zh-CN"),
        contact_name: ctx.body.contact_name ? String(ctx.body.contact_name) : undefined,
        contact_phone: ctx.body.contact_phone ? String(ctx.body.contact_phone) : undefined,
        contact_email: ctx.body.contact_email ? String(ctx.body.contact_email) : undefined,
        admin_email: adminEmail,
      })
      db.tenants.unshift(row)
      return { tenant: row, initial_password: randomPassword() }
    },
  },
  {
    method: "patch",
    pattern: "/api/platform/v1/tenants/:id",
    handler: (ctx) => {
      const e = requirePlatform(ctx); if (e) return e
      const t = db.tenants.find((x) => x.id === ctx.params.id)
      if (!t) return err(40401, "租户不存在")
      for (const key of ["name", "plan_code", "timezone", "locale", "contact_name", "contact_phone", "contact_email"] as const) {
        const value = ctx.body[key]
        if (value !== undefined) (t as unknown as Record<string, unknown>)[key] = value
      }
      if (ctx.body.expire_at !== undefined) {
        t.expire_at = String(ctx.body.expire_at)
        /* 续费：已到期的租户续到未来自动回到 active */
        if (t.status === "expired" && new Date(t.expire_at) > new Date()) t.status = "active"
      }
      t.updated_at = new Date().toISOString()
      return t
    },
  },
  {
    method: "patch",
    pattern: "/api/platform/v1/tenants/:id/status",
    handler: (ctx) => {
      const e = requirePlatform(ctx); if (e) return e
      const t = db.tenants.find((x) => x.id === ctx.params.id)
      if (!t) return err(40401, "租户不存在")
      const status = String(ctx.body.status || "")
      if (!["active", "suspended", "expired", "archived"].includes(status)) return err(40001, "非法状态")
      t.status = status as typeof t.status
      return t
    },
  },
  {
    method: "post",
    pattern: "/api/platform/v1/tenants/:id/retry",
    handler: (ctx) => {
      const e = requirePlatform(ctx); if (e) return e
      const t = db.tenants.find((x) => x.id === ctx.params.id)
      if (!t) return err(40401, "租户不存在")
      if (t.provision_status === "succeeded") return err(40001, "租户已开通成功，无需重试")
      const now = new Date().toISOString()
      t.provision_status = "succeeded"
      t.provision_step = "done"
      if (t.status === "pending") t.status = "active"
      t.updated_at = now
      t.provision_log = [
        ...t.provision_log,
        { step: "provision_retried", ok: true, at: now },
        { step: "tenant_bootstrapped", ok: true, at: now },
        { step: "done", ok: true, at: now },
      ]
      return { tenant: t, initial_password: randomPassword() }
    },
  },
  {
    method: "post",
    pattern: "/api/platform/v1/tenants/:id/admin-password",
    handler: (ctx) => {
      const e = requirePlatform(ctx); if (e) return e
      const t = db.tenants.find((x) => x.id === ctx.params.id)
      if (!t) return err(40401, "租户不存在")
      return { tenant: t, initial_password: randomPassword() }
    },
  },
  {
    method: "delete",
    pattern: "/api/platform/v1/tenants/:id",
    handler: (ctx) => {
      const e = requirePlatform(ctx); if (e) return e
      const t = db.tenants.find((x) => x.id === ctx.params.id)
      if (!t) return err(40401, "租户不存在")
      t.status = "archived"
      return { id: t.id, status: t.status }
    },
  },
  {
    method: "get",
    pattern: "/api/platform/v1/plans",
    handler: (ctx) => {
      const e = requirePlatform(ctx); if (e) return e
      return db.plans.map((p) => ({ ...p, tenant_count: db.tenants.filter((t) => t.plan_code === p.code).length }))
    },
  },
  {
    method: "post",
    pattern: "/api/platform/v1/plans",
    handler: (ctx) => {
      const e = requirePlatform(ctx); if (e) return e
      const code = String(ctx.body.code || "").trim().toLowerCase()
      const name = String(ctx.body.name || "").trim()
      if (!/^[a-z][a-z0-9_-]{1,31}$/.test(code)) return err(40001, "套餐编码格式不合法")
      if (!name) return err(40001, "套餐名称不能为空")
      if (db.plans.some((p) => p.code === code)) return err(40901, "套餐编码已存在")
      const now = new Date().toISOString()
      const plan = {
        id: randId("plan"), code, name,
        quota: (ctx.body.quota || proQuota) as typeof proQuota,
        enabled: ctx.body.enabled !== false,
        tenant_count: 0,
        created_at: now, updated_at: now,
      }
      db.plans.push(plan)
      return plan
    },
  },
  {
    method: "patch",
    pattern: "/api/platform/v1/plans/:id",
    handler: (ctx) => {
      const e = requirePlatform(ctx); if (e) return e
      const plan = db.plans.find((p) => p.id === ctx.params.id)
      if (!plan) return err(40401, "套餐不存在")
      if (ctx.body.name !== undefined) plan.name = String(ctx.body.name)
      if (ctx.body.enabled !== undefined) plan.enabled = Boolean(ctx.body.enabled)
      const q = ctx.body.quota as Record<string, number> | undefined
      if (q) {
        for (const k of ["max_users", "storage_gb", "api_qps", "mcp_calls", "mcp_tokens", "mcp_sessions"] as const) {
          if (q[k] !== undefined) plan.quota[k] = Number(q[k])
        }
        const features = (ctx.body.quota as { features?: string[] } | undefined)?.features
        if (features) plan.quota.features = features
      }
      plan.updated_at = new Date().toISOString()
      return { ...plan, tenant_count: db.tenants.filter((t) => t.plan_code === plan.code).length }
    },
  },
  {
    method: "get",
    pattern: "/api/platform/v1/audit-logs",
    handler: (ctx) => {
      const e = requirePlatform(ctx); if (e) return e
      let items = [...db.auditLogs]
      if (ctx.query.category) items = items.filter((l) => l.category === ctx.query.category)
      if (ctx.query.action) items = items.filter((l) => l.action === ctx.query.action)
      if (ctx.query.level) items = items.filter((l) => l.level === ctx.query.level)
      if (ctx.query.actor) items = items.filter((l) => (l.actor_email || "").includes(ctx.query.actor))
      if (ctx.query.from) items = items.filter((l) => l.created_at >= ctx.query.from)
      if (ctx.query.to) items = items.filter((l) => l.created_at <= ctx.query.to)
      items.sort((a, b) => b.created_at.localeCompare(a.created_at))
      return paged(items, ctx.query)
    },
  },

  /* ================= 租户侧 ================= */

  {
    method: "post",
    pattern: "/api/tenant/v1/auth/login",
    handler: ({ body }) => {
      const email = String(body.email || "").trim()
      const password = String(body.password || "")
      if (email !== "owner@acme.com" || password.length < 6) return err(40101, "邮箱或密码错误")
      db.tenantToken = "mock-tenant-token"
      return { access_token: "mock-tenant-token", expires_in: 900, need_change_password: false }
    },
  },
  {
    method: "get",
    pattern: "/api/tenant/v1/me",
    handler: (ctx) => {
      if (requireTenant(ctx)) return requireTenant(ctx)
      const acme = db.tenants[0]
      const owner = db.users[0]
      return {
        user: { id: owner.id, email: owner.email, display_name: owner.display_name, roles: owner.roles },
        tenant: { id: acme.id, name: acme.name, subdomain: acme.subdomain, plan_id: acme.plan_code, plan_name: acme.plan_name },
        permissions: tenantPermissions,
      }
    },
  },
  {
    method: "get",
    pattern: "/api/tenant/v1/stats/overview",
    handler: (ctx) => {
      const e = requireTenant(ctx); if (e) return e
      return { usage: { ...db.tenantUsage }, quota: proQuota, call_series: tenantCallSeries }
    },
  },
  {
    method: "get",
    pattern: "/api/tenant/v1/users",
    handler: (ctx) => {
      const e = requireTenant(ctx); if (e) return e
      let items = [...db.users]
      const kw = (ctx.query.keyword || "").toLowerCase()
      if (kw) items = items.filter((u) => u.email.includes(kw) || u.display_name.toLowerCase().includes(kw))
      if (ctx.query.status) items = items.filter((u) => u.status === ctx.query.status)
      return paged(items, ctx.query)
    },
  },
  {
    method: "post",
    pattern: "/api/tenant/v1/users",
    handler: (ctx) => {
      const e = requireTenant(ctx); if (e) return e
      const email = String(ctx.body.email || "").trim()
      const displayName = String(ctx.body.display_name || "").trim()
      const roles = (ctx.body.roles as string[] | undefined) ?? ["member"]
      if (!/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(email)) return err(40001, "邮箱格式不合法")
      if (!displayName) return err(40001, "姓名不能为空")
      if (db.users.some((u) => u.email === email)) return err(40901, "该邮箱已是租户成员")
      const user = {
        id: randId("u"), email, display_name: displayName,
        status: "active" as const, roles: roles.includes("admin") ? ["admin"] : ["member"],
        created_at: new Date().toISOString(), last_login_at: "",
      }
      db.users.push(user)
      db.tenantUsage.users += 1
      return { user, initial_password: randomPassword() }
    },
  },
  {
    method: "patch",
    pattern: "/api/tenant/v1/users/:id/status",
    handler: (ctx) => {
      const e = requireTenant(ctx); if (e) return e
      const user = db.users.find((u) => u.id === ctx.params.id)
      if (!user) return err(40401, "用户不存在")
      const status = String(ctx.body.status || "")
      if (!["active", "disabled"].includes(status)) return err(40001, "非法状态")
      user.status = status as typeof user.status
      return user
    },
  },
  {
    method: "delete",
    pattern: "/api/tenant/v1/users/:id",
    handler: (ctx) => {
      const e = requireTenant(ctx); if (e) return e
      const idx = db.users.findIndex((u) => u.id === ctx.params.id)
      if (idx < 0) return err(40401, "用户不存在")
      const user = db.users[idx]
      const remainingAdmins = db.users.filter((u, i) => i !== idx && u.status === "active" && u.roles.includes("admin")).length
      if (user.roles.includes("admin") && remainingAdmins === 0) return err(40009, "不能删除最后一个管理员，请先转移管理员角色")
      db.users.splice(idx, 1)
      db.tenantUsage.users = Math.max(0, db.tenantUsage.users - 1)
      return { id: user.id }
    },
  },
  {
    method: "get",
    pattern: "/api/tenant/v1/api-keys",
    handler: (ctx) => {
      const e = requireTenant(ctx); if (e) return e
      return paged(db.apiKeys, ctx.query)
    },
  },
  {
    method: "post",
    pattern: "/api/tenant/v1/api-keys",
    handler: (ctx) => {
      const e = requireTenant(ctx); if (e) return e
      const name = String(ctx.body.name || "").trim()
      if (!name) return err(40001, "名称不能为空")
      const secret = randomSecret()
      const key = {
        id: randId("k"), name, prefix: secret.slice(0, 14), owner_name: db.users[0].display_name,
        status: "active" as const, created_at: new Date().toISOString(), last_used_at: "",
      }
      db.apiKeys.unshift(key)
      return { key, secret }
    },
  },
  {
    method: "delete",
    pattern: "/api/tenant/v1/api-keys/:id",
    handler: (ctx) => {
      const e = requireTenant(ctx); if (e) return e
      const key = db.apiKeys.find((k) => k.id === ctx.params.id)
      if (!key) return err(40401, "Key 不存在")
      key.status = "revoked"
      return { id: key.id }
    },
  },
  {
    method: "get",
    pattern: "/api/tenant/v1/mcp/servers",
    handler: (ctx) => {
      const e = requireTenant(ctx); if (e) return e
      return db.mcpServers
    },
  },
  {
    method: "get",
    pattern: "/api/tenant/v1/mcp/tools",
    handler: (ctx) => {
      const e = requireTenant(ctx); if (e) return e
      let items = [...db.mcpTools]
      if (ctx.query.server_id) items = items.filter((t) => t.server_id === ctx.query.server_id)
      return paged(items, ctx.query)
    },
  },
  {
    method: "patch",
    pattern: "/api/tenant/v1/mcp/tools/:id",
    handler: (ctx) => {
      const e = requireTenant(ctx); if (e) return e
      const tool = db.mcpTools.find((t) => t.id === ctx.params.id)
      if (!tool) return err(40401, "工具不存在")
      if (ctx.body.enabled !== undefined) tool.enabled = Boolean(ctx.body.enabled)
      return tool
    },
  },
]
