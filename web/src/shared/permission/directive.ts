import type { App, Directive } from "vue";
import { useAuthStore } from "@/shared/auth/store";

/* v-permission="'tenant.user:create'"：无权限直接移除节点（指令形式只用于按钮显隐；
 * 菜单 / 路由级控制在 router guard 与 SideNav 过滤） */

export const permissionDirective: Directive<HTMLElement, string | undefined> = {
  mounted(el, binding) {
    const auth = useAuthStore();
    if (!auth.can(binding.value)) {
      el.parentNode?.removeChild(el);
    }
  },
};

export function setupPermission(app: App) {
  app.directive("permission", permissionDirective);
}
