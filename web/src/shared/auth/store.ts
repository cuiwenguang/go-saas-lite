import { defineStore } from "pinia";
import { computed, ref } from "vue";
import {
  fetchPlatformMe, fetchTenantMe, loginPlatform, loginTenant, logoutPlatform, logoutTenant,
} from "@/shared/api/auth";
import type { PlatformRole } from "@/shared/api/types";
import { clearTenant, setCurrentTenant } from "@/shared/tenant/context";

/* 登录态：access token 只存内存（CONVENTIONS-FRONTEND §5，不落 localStorage）
 * 平台 / 租户两套身份物理隔离，登录与 /me 走各自接口 */

export const useAuthStore = defineStore("auth", () => {
  const token = ref("");
  const displayName = ref("");
  const permissions = ref<string[]>([]);
  /** 仅平台侧有值：owner / viewer */
  const role = ref<PlatformRole | "">("");
  /** 仅租户侧有值：当前租户上下文 */
  const tenant = ref<{ id: string; name: string; subdomain: string; plan_code: string } | null>(null);

  const isAuthed = computed(() => token.value !== "");

  function setToken(next: string) {
    token.value = next;
  }

  async function platformLogin(email: string, password: string) {
    const r = await loginPlatform(email, password);
    token.value = r.access_token;
    role.value = r.role;
    await loadPlatformMe();
  }

  async function tenantLogin(email: string, password: string) {
    const r = await loginTenant(email, password);
    token.value = r.access_token;
    await loadTenantMe();

    return r.must_change_password;
  }

  async function loadPlatformMe() {
    const me = await fetchPlatformMe();
    displayName.value = me.user.email;
    role.value = me.role;
    permissions.value = me.permissions;
    tenant.value = null;
  }

  async function loadTenantMe() {
    const me = await fetchTenantMe();
    displayName.value = me.user.display_name;
    permissions.value = me.permissions;
    tenant.value = {
      id: me.tenant.id,
      name: me.tenant.name,
      subdomain: me.tenant.subdomain || "",
      plan_code: me.tenant.plan_code,
    };
    setCurrentTenant(me.tenant.id);
  }

  /** 权限点判断：无 perm 参数视为公开 */
  function can(perm?: string) {
    return !perm || permissions.value.includes(perm);
  }

  /** 登出：先尽量作废服务端 refresh 令牌，本地状态无论如何都要清掉 */
  async function signOut() {
    try {
      await logoutPlatform();
    } catch {
      /* 服务端不可用时也必须能本地登出 */
    }
    logout();
  }

  /** 租户登出：刷新令牌在 httpOnly cookie，必须显式通知服务端 */
  async function signOutTenant() {
    try {
      await logoutTenant();
    } catch {
      /* 同上 */
    }
    logout();
  }

  function logout() {
    token.value = "";
    displayName.value = "";
    permissions.value = [];
    role.value = "";
    tenant.value = null;
    clearTenant();
  }

  return {
    token, displayName, permissions, role, tenant, isAuthed,
    setToken, platformLogin, tenantLogin, loadPlatformMe, loadTenantMe, can, signOut, signOutTenant, logout,
  };
});
