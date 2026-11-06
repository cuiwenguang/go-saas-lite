import { createRouter, createWebHistory, type RouteRecordRaw } from "vue-router";
import AppShell from "@/shared/layout/AppShell.vue";
import { useAuthStore } from "@/shared/auth/store";
import { PERM } from "@/shared/permission/perms";

const routes: RouteRecordRaw[] = [
  {
    path: "/admin/login",
    name: "login",
    component: () => import("@/views/admin/login/index.vue"),
    meta: { public: true },
  },
  {
    path: "/admin",
    component: AppShell,
    props: { side: "admin" },
    children: [
      { path: "", redirect: { name: "dashboard" } },
      {
        path: "dashboard",
        name: "dashboard",
        component: () => import("@/views/admin/dashboard/index.vue"),
        meta: { title: "总览", icon: "Odometer" },
      },
      {
        path: "tenants",
        name: "tenants",
        component: () => import("@/views/admin/tenants/index.vue"),
        meta: { title: "租户管理", icon: "OfficeBuilding", perm: PERM.tenantRead },
      },
      {
        path: "tenants/:id",
        name: "tenant-detail",
        component: () => import("@/views/admin/tenants/detail.vue"),
        meta: { title: "租户详情", menu: false, perm: PERM.tenantRead },
      },
      {
        path: "plans",
        name: "plans",
        component: () => import("@/views/admin/plans/index.vue"),
        meta: { title: "套餐管理", icon: "Tickets", perm: PERM.planRead },
      },
      {
        path: "audit",
        name: "audit",
        component: () => import("@/views/admin/audit/index.vue"),
        meta: { title: "审计日志", icon: "Memo", perm: PERM.auditRead },
      },
    ],
  },
  { path: "/admin/:pathMatch(.*)*", redirect: { name: "dashboard" } },
];

export const router = createRouter({
  /* 注意：路由表路径已含 /admin 前缀，历史模式不能再传 base，否则双重前缀导致路由失配（白屏无报错） */
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
    return { name: "login", query: to.fullPath !== "/admin/" ? { redirect: to.fullPath } : undefined };
  }
  if (to.meta.perm && !auth.can(String(to.meta.perm))) {
    ElMessage.warning("没有访问该页面的权限");
    return { name: "dashboard" };
  }
  return true;
});
