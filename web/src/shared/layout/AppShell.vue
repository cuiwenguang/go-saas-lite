<script setup lang="ts">
import SideNav from "./SideNav.vue";
import Topbar from "./Topbar.vue";
import { useAppStore } from "@/shared/stores/app";

defineProps<{ side: "admin" | "tenant" }>();
const app = useAppStore();
</script>

<template>
  <el-container class="shell">
    <el-aside :width="app.collapsed ? '64px' : '224px'" class="shell-aside">
      <SideNav :side="side" />
    </el-aside>
    <el-container>
      <el-header height="56px" class="shell-header">
        <Topbar :side="side" />
      </el-header>
      <el-main class="shell-main">
        <router-view v-slot="{ Component }">
          <transition name="page" mode="out-in">
            <component :is="Component" />
          </transition>
        </router-view>
      </el-main>
    </el-container>
  </el-container>
</template>

<style scoped>
.shell {
  height: 100vh;
}
.shell-aside {
  background: var(--el-bg-color);
  transition: width 0.2s ease;
  overflow: hidden;
  position: relative;
}
/* 竖向分割线通高（header 透明无边框，不存在交叉重叠） */
.shell-aside::after {
  content: "";
  position: absolute;
  top: 0;
  bottom: 0;
  right: 0;
  width: 1px;
  background: var(--el-border-color-lighter);
}
.shell-header {
  padding: 0;
}
.shell-main {
  padding: 0;
  overflow-y: auto;
}

/* 路由切换：内容上移 8px + 淡入 */
.page-enter-from {
  opacity: 0;
  transform: translateY(8px);
}
.page-enter-active {
  transition: opacity 0.18s ease, transform 0.18s ease;
}
.page-leave-to {
  opacity: 0;
}
.page-leave-active {
  transition: opacity 0.1s ease;
}
</style>
