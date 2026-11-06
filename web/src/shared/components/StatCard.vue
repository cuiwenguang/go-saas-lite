<script setup lang="ts">
import type { Component } from "vue";
import { computed, onMounted, ref, watch } from "vue";

/* 编辑级统计卡：大数字为视觉主角（方向 B）
 * 数字滚动：value 为 number 时挂载/更新从 0 滚到目标值（600ms ease-out） */
const props = defineProps<{
  label: string
  value: string | number
  sub?: string
  icon?: Component
  /** 0-100，配额类统计显示细进度线 */
  progress?: number
  /** 超过阈值变警示色 */
  danger?: boolean
}>();

const fmt = new Intl.NumberFormat("zh-CN");

function animateTo(target: number) {
  const start = performance.now();
  const dur = 600;
  const tick = (now: number) => {
    const t = Math.min(1, (now - start) / dur);
    const eased = 1 - Math.pow(1 - t, 3);
    shown.value = Math.round(target * eased);
    if (t < 1) requestAnimationFrame(tick);
  };
  requestAnimationFrame(tick);
}

const shown = ref(0);
const isNumber = computed(() => typeof props.value === "number");

onMounted(() => {
  if (isNumber.value) animateTo(Number(props.value));
});

watch(
  () => props.value,
  (v) => {
    if (typeof v === "number") animateTo(v);
  },
);

const display = computed(() =>
  isNumber.value ? fmt.format(shown.value) : String(props.value),
);

const progressColor = computed(() => {
  if (props.danger) return "var(--el-color-danger)";
  if ((props.progress ?? 0) >= 90) return "var(--el-color-warning)";
  return "var(--brand)";
});
</script>

<template>
  <div class="stat">
    <p class="stat-label">
      <el-icon v-if="icon" :size="14" class="stat-label-icon"><component :is="icon" /></el-icon>
      {{ label }}
    </p>
    <p class="stat-value num">{{ display }}</p>

    <template v-if="progress !== undefined">
      <div class="stat-progress">
        <div class="stat-progress-track">
          <div class="stat-progress-fill" :style="{ width: `${Math.min(100, progress)}%`, background: progressColor }"></div>
        </div>
        <span class="stat-progress-text num" :style="{ color: progressColor }">{{ progress }}%</span>
      </div>
    </template>
    <p v-if="sub" class="stat-sub">{{ sub }}</p>
  </div>
</template>

<style scoped>
.stat {
  background: var(--el-bg-color);
  border-radius: 12px;
  padding: 18px 20px 16px;
}
.stat-label {
  margin: 0;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  display: flex;
  align-items: center;
  gap: 6px;
}
.stat-label-icon {
  opacity: 0.7;
}
.stat-value {
  margin: 8px 0 0;
  font-size: 30px;
  font-weight: 500;
  letter-spacing: -0.5px;
  color: var(--el-text-color-primary);
  line-height: 1.15;
}
.stat-sub {
  margin: 6px 0 0;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.stat-progress {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: 10px;
}
.stat-progress-track {
  flex: 1;
  height: 4px;
  border-radius: 2px;
  background: var(--el-fill-color);
  overflow: hidden;
}
.stat-progress-fill {
  height: 100%;
  border-radius: 2px;
  transition: width 0.4s ease;
}
.stat-progress-text {
  font-size: 12px;
  min-width: 34px;
  text-align: right;
}
</style>
