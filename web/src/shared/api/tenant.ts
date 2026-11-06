import { del, get, patch, post } from "@/shared/http/client";
import type {
  ApiKeyRow, CreateApiKeyResult, InviteUserResult, McpServerRow, McpToolRow, Paged, TenantStats, UserRow,
} from "./types";

/* 租户侧：用户 / API Key / MCP */

export function listUsers(params: { page?: number; page_size?: number; status?: string; keyword?: string }) {
  return get<Paged<UserRow>>("/api/tenant/v1/users", params);
}

export function inviteUser(body: { email: string; display_name: string; roles?: string[] }) {
  return post<InviteUserResult>("/api/tenant/v1/users", body);
}

export function setUserStatus(id: string, status: "active" | "disabled") {
  return patch<UserRow>(`/api/tenant/v1/users/${id}/status`, { status });
}

export function removeUser(id: string) {
  return del<{ id: string }>(`/api/tenant/v1/users/${id}`);
}

export function listApiKeys(params?: { page?: number; page_size?: number }) {
  return get<Paged<ApiKeyRow>>("/api/tenant/v1/api-keys", params);
}

export function createApiKey(name: string) {
  return post<CreateApiKeyResult>("/api/tenant/v1/api-keys", { name });
}

export function revokeApiKey(id: string) {
  return del<{ id: string }>(`/api/tenant/v1/api-keys/${id}`);
}

export function listMcpServers() {
  return get<McpServerRow[]>("/api/tenant/v1/mcp/servers");
}

export function listMcpTools(params?: { page?: number; page_size?: number; server_id?: string }) {
  return get<Paged<McpToolRow>>("/api/tenant/v1/mcp/tools", params);
}

export function setToolEnabled(id: string, enabled: boolean) {
  return patch<McpToolRow>(`/api/tenant/v1/mcp/tools/${id}`, { enabled });
}

export function tenantStats() {
  return get<TenantStats>("/api/tenant/v1/stats/overview");
}
