/* 权限点常量（CONVENTIONS-FRONTEND §5）
 * 与后端 authz.All() 的权限点命名同源：资源 + 动作
 * 禁止在业务代码里直接写权限字符串，一律引用此处常量 */

export const PERM = {
  /* 平台侧 */
  tenantRead: "tenant:read",
  tenantCreate: "tenant:create",
  tenantUpdate: "tenant:update",
  tenantDelete: "tenant:delete",
  planRead: "plan:read",
  planUpdate: "plan:update",
  statsRead: "stats:read",
  auditRead: "audit:read",

  /* 租户侧 */
  userRead: "user:read",
  userCreate: "user:create",
  userUpdate: "user:update",
  userDelete: "user:delete",
  roleRead: "role:read",
  apiKeyRead: "apikey:read",
  apiKeyCreate: "apikey:create",
  apiKeyRevoke: "apikey:revoke",
  mcpRead: "mcp:read",
  mcpUpdate: "mcp:update",
  oauthGrantRead: "oauth.grant:read",
  oauthGrantRevoke: "oauth.grant:revoke",
  tenantConfRead: "tenantconf:read",
  tenantConfUpdate: "tenantconf:update",
} as const;

export type Perm = (typeof PERM)[keyof typeof PERM];
