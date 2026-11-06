<script setup lang="ts">
import { Fold, Expand } from "@element-plus/icons-vue";
import QuickActions from "./QuickActions.vue";
import { useAppStore } from "@/shared/stores/app";
import { useProgress } from "@/shared/utils/progress";

const props = defineProps<{ side: "admin" | "tenant" }>();

const route = useRoute();
const app = useAppStore();
const { progress, visible } = useProgress();

const rootPath = props.side === "admin" ? "/admin" : "/app";
const rootTitle = props.side === "admin" ? "平台管理" : "租户控制台";
const crumbs = computed(() =>
  route.matched
    .filter((r) => r.meta?.title && r.path !== "/admin" && r.path !== "/app")
    .map((r) => String(r.meta!.title)),
);
</script>

<template>
  <div class="topbar">
    <div v-show="visible" class="req-bar" :style="{ width: `${progress}%` }"></div>
    <div class="topbar-left">
      <el-tooltip :content="app.collapsed ? '展开导航' : '折叠导航'" placement="bottom">
        <el-button :icon="app.collapsed ? Expand : Fold" text class="collapse-btn" @click="app.toggleCollapsed()" />
      </el-tooltip>
      <span class="left-divider"></span>
      <el-breadcrumb separator="/">
        <el-breadcrumb-item :to="rootPath">{{ rootTitle }}</el-breadcrumb-item>
        <el-breadcrumb-item v-for="c in crumbs" :key="c">{{ c }}</el-breadcrumb-item>
      </el-breadcrumb>
    </div>

    <div class="topbar-right">
      <QuickActions :side="side" />
    </div>
  </div>
</template>

<style scoped>
.topbar {
  display: flex;
  align-items: center;
  height: 56px;
  background: transparent;
  padding: 0 16px;
  position: relative;
}
.req-bar {
  position: fixed;
  top: 0;
  left: 0;
  height: 2px;
  background: var(--brand);
  border-radius: 0 2px 2px 0;
  transition: width 0.25s ease;
  z-index: 2000;
}
.topbar-left {
  display: flex;
  align-items: center;
  gap: 14px;
  min-width: 0;
}
.collapse-btn {
  width: 34px;
  height: 34px;
  margin: 0;
  border-radius: 9px;
}
.left-divider {
  width: 1px;
  height: 16px;
  background: var(--el-border-color);
}
.topbar-left :deep(.el-breadcrumb) {
  font-size: 13px;
  white-space: nowrap;
}
.topbar-left :deep(.el-breadcrumb__item:last-child .el-breadcrumb__inner) {
  color: var(--el-text-color-primary);
  font-weight: 500;
}
.topbar-right {
  margin-left: auto;
  display: flex;
  align-items: center;
}
</style>
