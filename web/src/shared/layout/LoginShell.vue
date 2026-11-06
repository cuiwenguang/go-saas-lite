<script setup lang="ts">
import { Check } from "@element-plus/icons-vue";

defineProps<{
  productName: string
  tagline: string
  bullets: string[]
  footer?: string
}>();
</script>

<template>
  <div class="login-wrap">
    <div class="login-brand">
      <div class="brand-mark">S</div>
      <h1 class="brand-title">{{ productName }}</h1>
      <p class="brand-tagline">{{ tagline }}</p>
      <ul class="brand-list">
        <li v-for="b in bullets" :key="b">
          <el-icon :size="14"><Check /></el-icon>
          <span>{{ b }}</span>
        </li>
      </ul>
      <p class="brand-footer">{{ footer }}</p>
    </div>
    <div class="login-panel">
      <slot />
    </div>
  </div>
</template>

<style scoped>
.login-wrap {
  min-height: 100vh;
  display: grid;
  grid-template-columns: minmax(360px, 44%) 1fr;
}
.login-brand {
  background: var(--brand);
  color: #fff;
  padding: 56px 56px 32px;
  display: flex;
  flex-direction: column;
}
.brand-mark {
  width: 44px;
  height: 44px;
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.18);
  font-size: 20px;
  font-weight: 500;
  display: flex;
  align-items: center;
  justify-content: center;
}
.brand-title {
  margin: 22px 0 0;
  font-size: 26px;
  font-weight: 500;
}
.brand-tagline {
  margin: 10px 0 0;
  font-size: 14px;
  opacity: 0.85;
  line-height: 1.7;
}
.brand-list {
  list-style: none;
  margin: 28px 0 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.brand-list li {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 13px;
  opacity: 0.92;
}
.brand-footer {
  margin-top: auto;
  font-size: 12px;
  opacity: 0.6;
}
.login-panel {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 32px;
  background: var(--page-bg);
}

@media (max-width: 768px) {
  .login-wrap {
    grid-template-columns: 1fr;
  }
  .login-brand {
    display: none;
  }
}
</style>
