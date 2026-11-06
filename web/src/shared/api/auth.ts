import { get, post } from "@/shared/http/client";
import type { PlatformMe, PlatformSession, TenantMe } from "./types";

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

export async function loginTenant(email: string, password: string) {
  return post<{ access_token: string; expires_in: number; need_change_password: boolean }>(
    "/api/tenant/v1/auth/login",
    { email, password },
  );
}

export async function fetchTenantMe() {
  return get<TenantMe>("/api/tenant/v1/me");
}
