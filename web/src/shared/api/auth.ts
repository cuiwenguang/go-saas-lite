import { get, post } from "@/shared/http/client";
import type { PlatformMe, PlatformSession, TenantMe, TenantSession } from "./types";

/* 认证：平台与租户两套体系物理隔离（readme D7：平台 token 不能访问租户接口，反之亦然）
 * refresh 令牌走 httpOnly cookie，前端不接触；401 由 shared/http 透明刷新 */

export async function loginPlatform(email: string, password: string) {
  return post<PlatformSession>("/api/platform/v1/auth/login", { email, password });
}

export async function refreshPlatformSession() {
  return post<PlatformSession>("/api/platform/v1/auth/refresh");
}

export async function logoutPlatform() {
  return post<{ ok: boolean }>("/api/platform/v1/auth/logout");
}

export async function changePlatformPassword(oldPassword: string, newPassword: string) {
  return post<{ ok: boolean }>("/api/platform/v1/auth/password", {
    old_password: oldPassword,
    new_password: newPassword,
  });
}

export async function fetchPlatformMe() {
  return get<PlatformMe>("/api/platform/v1/me");
}

/* ---------- 租户会话 ----------
 * 租户由子域名 / X-Tenant-Id 确定（见 shared/tenant/context.ts），登录入参不需要租户；
 * 刷新与登出只认 httpOnly cookie。 */

export async function loginTenant(email: string, password: string) {
  return post<TenantSession>("/api/tenant/v1/auth/login", { email, password });
}

export async function refreshTenantSession() {
  return post<TenantSession>("/api/tenant/v1/auth/refresh");
}

export async function logoutTenant() {
  return post<{ ok: boolean }>("/api/tenant/v1/auth/logout");
}

export async function changeTenantPassword(oldPassword: string, newPassword: string) {
  return post<{ ok: boolean }>("/api/tenant/v1/auth/password", {
    old_password: oldPassword,
    new_password: newPassword,
  });
}

export async function fetchTenantMe() {
  return get<TenantMe>("/api/tenant/v1/me");
}
