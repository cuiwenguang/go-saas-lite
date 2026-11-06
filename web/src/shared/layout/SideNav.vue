<script setup lang="ts">
import type { Component } from "vue";
import { ArrowDown, Connection, Key, Lock, Memo, OfficeBuilding, Odometer, Setting, Tickets, User } from "@element-plus/icons-vue";
import { ElMessage } from "element-plus";
import { useAuthStore } from "@/shared/auth/store";
import { useAppStore } from "@/shared/stores/app";

const props = defineProps<{ side: "admin" | "tenant" }>();

const icons: Record<string, Component> = {
  Odometer, OfficeBuilding, Tickets, Memo, User, Key, Connection, Lock, Setting,
};

const route = useRoute();
const router = useRouter();
const auth = useAuthStore();
const app = useAppStore();

const rootPath = props.side === "admin" ? "/admin" : "/app";

interface MenuItem { path: string; title: string; icon?: string; perm?: string }

const menus = computed<MenuItem[]>(() => {
  const root = router.options.routes.find((r) => r.path === rootPath);
  const children = (root?.children || []) as { path: string; meta?: Record<string, unknown> }[];
  return children
    .filter((r) => r.meta?.title && r.meta?.menu !== false)
    .map((r) => ({
      path: `${rootPath}/${r.path}`.replace(/\/+$/, "") || rootPath,
      title: String(r.meta!.title),
      icon: r.meta?.icon ? String(r.meta.icon) : undefined,
      perm: r.meta?.perm ? String(r.meta.perm) : undefined,
    }))
    .filter((m) => auth.can(m.perm));
});

const roleText = computed(() => props.side === "admin" ? "平台管理员" : `${auth.tenant?.plan_code || "默认"} 套餐`);

function onLogout() {
  const signOut = props.side === "admin" ? auth.signOut() : auth.signOutTenant();
  void signOut.finally(() => router.push({ name: "login" }));
}
</script>

<template>
  <div class="side-nav">
    <div class="brand" :class="{ collapsed: app.collapsed }">
      <div class="brand-mark">S</div>
      <div v-if="!app.collapsed" class="brand-text">
        <template v-if="side === 'admin'">
          <p class="brand-name">go-saas-lite</p>
          <p class="brand-sub">平台管理</p>
        </template>
        <template v-else>
          <p class="brand-name">{{ auth.tenant?.name || "租户控制台" }}</p>
          <p class="brand-sub">{{ auth.tenant?.subdomain || "go-saas-lite" }}</p>
        </template>
      </div>
    </div>

    <nav class="nav-wrap" aria-label="主导航">
      <el-menu :default-active="route.path" :collapse="app.collapsed" :collapse-transition="false" router>
        <el-menu-item v-for="m in menus" :key="m.path" :index="m.path">
          <el-icon v-if="m.icon"><component :is="icons[m.icon]" /></el-icon>
          <template #title>{{ m.title }}</template>
        </el-menu-item>
      </el-menu>
    </nav>

    <div class="user-section" :class="{ collapsed: app.collapsed }">
      <el-dropdown trigger="click" :placement="app.collapsed ? 'top' : 'top-start'">
        <button class="user-entry" type="button" :title="app.collapsed ? auth.displayName : undefined">
          <el-avatar :size="32" class="user-avatar">{{ (auth.displayName || "?").slice(0, 1) }}</el-avatar>
          <span v-if="!app.collapsed" class="user-meta">
            <span class="user-name">{{ auth.displayName || "未登录" }}</span>
            <span class="user-role">{{ roleText }}</span>
          </span>
          <el-icon v-if="!app.collapsed" class="user-arrow" :size="12"><ArrowDown /></el-icon>
        </button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item @click="ElMessage.info('账号设置将在后续版本提供')">账号设置</el-dropdown-item>
            <el-dropdown-item divided @click="onLogout">退出登录</el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
    </div>
  </div>
</template>

<style scoped>
.side-nav {
  height: 100%;
  display: flex;
  flex-direction: column;
}
.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 56px;
  flex-shrink: 0;
  padding: 0 16px;
  border-bottom: 1px solid var(--el-border-color-lighter);
}
.brand.collapsed {
  justify-content: center;
  padding: 0;
}
.brand-mark {
  width: 34px;
  height: 34px;
  border-radius: 10px;
  background: var(--brand);
  color: #fff;
  font-weight: 500;
  font-size: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.brand-text {
  min-width: 0;
}
.brand-name {
  margin: 0;
  font-size: 14px;
  font-weight: 500;
  color: var(--el-text-color-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 130px;
}
.brand-sub {
  margin: 2px 0 0;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  white-space: nowrap;
}
.nav-wrap {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
}
:deep(.el-menu) {
  padding: 10px 0;
}
.user-section {
  flex-shrink: 0;
  padding: 10px 12px;
  border-top: 1px solid var(--el-border-color-lighter);
}
.user-section :deep(.el-dropdown) {
  width: 100%;
}
.user-entry {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px;
  border: 0;
  border-radius: 10px;
  background: transparent;
  color: inherit;
  text-align: left;
  cursor: pointer;
}
.user-entry:hover {
  background: var(--el-fill-color-light);
}
.user-avatar {
  flex-shrink: 0;
  background: var(--el-color-primary-light-8);
  color: var(--brand);
  font-size: 13px;
}
.user-meta {
  min-width: 0;
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 2px;
}
.user-name {
  overflow: hidden;
  color: var(--el-text-color-primary);
  font-size: 13px;
  font-weight: 500;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.user-role {
  color: var(--el-text-color-secondary);
  font-size: 11px;
}
.user-arrow {
  color: var(--el-text-color-secondary);
}
.user-section.collapsed {
  padding: 10px 8px;
}
.user-section.collapsed .user-entry {
  justify-content: center;
  padding: 8px 0;
}
</style>
