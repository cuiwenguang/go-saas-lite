import { del, get, patch, post, put } from "@/shared/http/client";
import type {
  ApiKeyRow, ConfigRow, CreateApiKeyResult, InviteUserResult, McpCallRow, McpServerRow, McpToolRow,
  OAuthGrantRow, Paged, TenantRole, TenantStats, UserRow,
} from "./types";

/* 租户侧：成员 / 角色 / API Key / MCP 配置 / 调用日志 / OAuth 授权 / 总览 */

/* ---------- 成员与角色 ---------- */

export function listUsers(params: { page?: number; size?: number; status?: string; keyword?: string }) {
  return get<Paged<UserRow>>("/api/tenant/v1/users", params);
}

export function createUser(body: { email: string; display_name: string; password?: string; role?: "admin" | "member" }) {
  return post<InviteUserResult>("/api/tenant/v1/users", body);
}

export function updateUser(id: string, body: { display_name?: string; status?: "active" | "disabled" }) {
  return patch<UserRow>(`/api/tenant/v1/users/${id}`, body);
}

export function resetUserPassword(id: string) {
  return post<{ initial_password: string }>(`/api/tenant/v1/users/${id}/password`);
}

export function setUserRoles(id: string, codes: string[]) {
  return patch<UserRow>(`/api/tenant/v1/users/${id}/roles`, { codes });
}

export function listRoles() {
  return get<{ items: TenantRole[] }>("/api/tenant/v1/roles");
}

/* ---------- API Key ---------- */

export function listApiKeys() {
  return get<{ items: ApiKeyRow[] }>("/api/tenant/v1/api-keys");
}

export function createApiKey(name: string) {
  return post<CreateApiKeyResult>("/api/tenant/v1/api-keys", { name });
}

export function revokeApiKey(id: string) {
  return del<{ revoked: boolean }>(`/api/tenant/v1/api-keys/${id}`);
}

/* ---------- MCP 服务与工具 ---------- */

export function listMcpServers() {
  return get<{ items: McpServerRow[] }>("/api/tenant/v1/mcp/servers");
}

export function createMcpServer(body: { name: string; base_url: string; credential?: string; enabled?: boolean }) {
  return post<McpServerRow>("/api/tenant/v1/mcp/servers", body);
}

export function updateMcpServer(
  id: string,
  body: { name?: string; base_url?: string; credential?: string; enabled?: boolean },
) {
  return patch<McpServerRow>(`/api/tenant/v1/mcp/servers/${id}`, body);
}

export function deleteMcpServer(id: string) {
  return del<{ deleted: boolean }>(`/api/tenant/v1/mcp/servers/${id}`);
}

export function listMcpTools(params?: { server_id?: string }) {
  return get<{ items: McpToolRow[] }>("/api/tenant/v1/mcp/tools", params);
}

export function updateMcpTool(id: string, body: { enabled?: boolean; required_perms?: string[] }) {
  return patch<McpToolRow>(`/api/tenant/v1/mcp/tools/${id}`, body);
}

export function listMcpCalls(params: { page?: number; size?: number; status?: string; tool?: string }) {
  return get<Paged<McpCallRow>>("/api/tenant/v1/mcp/calls", params);
}

/* ---------- OAuth 授权 ---------- */

export function listGrants(includeRevoked = false) {
  return get<{ items: OAuthGrantRow[] }>("/api/tenant/v1/oauth/grants", {
    include_revoked: includeRevoked ? 1 : undefined,
  });
}

export function revokeGrant(clientId: string) {
  return del<{ revoked: boolean }>(`/api/tenant/v1/oauth/grants/${clientId}`);
}

/* ---------- 租户配置 ---------- */

export function listConfigs(namespace: string) {
  return get<{ items: ConfigRow[] }>("/api/tenant/v1/configs", { namespace });
}

export function setConfig(namespace: string, key: string, value: string) {
  return put<ConfigRow>(`/api/tenant/v1/configs/${namespace}/${key}`, { value });
}

export function removeConfig(namespace: string, key: string) {
  return del<{ deleted: boolean }>(`/api/tenant/v1/configs/${namespace}/${key}`);
}

/* ---------- 总览 ---------- */

export function tenantStats() {
  return get<TenantStats>("/api/tenant/v1/stats/overview");
}
