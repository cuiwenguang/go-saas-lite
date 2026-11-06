import { createRouter, createWebHistory, type RouteRecordRaw } from "vue-router";
import AppShell from "@/shared/layout/AppShell.vue";
import { useAuthStore } from "@/shared/auth/store";
import { PERM } from "@/shared/permission/perms";

const routes: RouteRecordRaw[] = [
  {
    path: "/app/login",
    name: "login",
    component: () => import("@/views/tenant/login/index.vue"),
    meta: { public: true },
  },
  {
    path: "/app",
    component: AppShell,
    props: { side: "tenant" },
    children: [
      { path: "", redirect: { name: "dashboard" } },
      {
        path: "dashboard",
        name: "dashboard",
        component: () => import("@/views/tenant/dashboard/index.vue"),
        meta: { title: "总览", icon: "Odometer" },
      },
      {
        path: "users",
        name: "users",
        component: () => import("@/views/tenant/users/index.vue"),
        meta: { title: "成员管理", icon: "User", perm: PERM.userRead },
      },
      {
        path: "api-keys",
        name: "api-keys",
        component: () => import("@/views/tenant/apikeys/index.vue"),
        meta: { title: "API Key", icon: "Key", perm: PERM.apiKeyRead },
      },
      {
        path: "mcp",
        name: "mcp",
        component: () => import("@/views/tenant/mcp/index.vue"),
        meta: { title: "MCP 服务", icon: "Connection", perm: PERM.mcpRead },
      },
      {
        path: "grants",
        name: "grants",
        component: () => import("@/views/tenant/grants/index.vue"),
        meta: { title: "Agent 授权", icon: "Lock", perm: PERM.oauthGrantRead },
      },
      {
        path: "settings",
        name: "settings",
        component: () => import("@/views/tenant/settings/index.vue"),
        meta: { title: "租户配置", icon: "Setting", perm: PERM.tenantConfRead },
      },
    ],
  },
  { path: "/app/:pathMatch(.*)*", redirect: { name: "dashboard" } },
];

export const router = createRouter({
  /* 注意：路由表路径已含 /app 前缀，历史模式不能再传 base，否则双重前缀导致路由失配（白屏无报错） */
  history: createWebHistory(),
  routes,
});

router.beforeEach((to) => {
  const auth = useAuthStore();
  if (to.meta.public) {
    if (to.name === "login" && auth.isAuthed) return { name: "dashboard" };
    return true;
  }
  if (!auth.isAuthed) {
    return { name: "login", query: to.fullPath !== "/app/" ? { redirect: to.fullPath } : undefined };
  }
  if (to.meta.perm && !auth.can(String(to.meta.perm))) {
    ElMessage.warning("没有访问该页面的权限");
    return { name: "dashboard" };
  }
  return true;
});
