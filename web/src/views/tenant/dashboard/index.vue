<script setup lang="ts">
import { Coin, Connection, DataLine, User } from "@element-plus/icons-vue";
import { tenantStats } from "@/shared/api/tenant";
import { cssVar, fmtNumber, pct } from "@/shared/utils/format";
import StatCard from "@/shared/components/StatCard.vue";
import Chart from "@/shared/components/Chart.vue";
import { useAuthStore } from "@/shared/auth/store";
import type { TenantStats } from "@/shared/api/types";
import type { EChartsCoreOption } from "echarts/core";

const auth = useAuthStore();
const stats = ref<TenantStats | null>(null);

onMounted(async () => {
  stats.value = await tenantStats();
});

const cards = computed(() => {
  if (!stats.value) return [];
  const { usage, quota } = stats.value;
  return [
    { label: "成员", value: usage.users, sub: `上限 ${quota.max_users || "不限"}`, icon: User, progress: pct(usage.users, quota.max_users) },
    { label: "MCP 调用（本月）", value: usage.mcp_calls, sub: `上限 ${quota.mcp_calls ? fmtNumber(quota.mcp_calls) : "不限"}`, icon: Connection, progress: pct(usage.mcp_calls, quota.mcp_calls) },
    { label: "MCP Tokens", value: usage.mcp_tokens, sub: `上限 ${quota.mcp_tokens ? fmtNumber(quota.mcp_tokens) : "不限"}`, icon: Coin, progress: pct(usage.mcp_tokens, quota.mcp_tokens) },
    { label: "存储 (GB)", value: usage.storage_gb, sub: `上限 ${quota.storage_gb || "不限"}`, icon: DataLine, progress: pct(usage.storage_gb, quota.storage_gb) },
  ];
});

const chartOption = computed<EChartsCoreOption | null>(() => {
  if (!stats.value) return null;
  const text = cssVar("--el-text-color-secondary");
  const line = cssVar("--el-border-color-light");
  return {
    grid: { left: 55, right: 16, top: 24, bottom: 28 },
    tooltip: { trigger: "axis" },
    xAxis: {
      type: "category",
      data: stats.value.call_series.map((p) => p.date),
      axisLine: { lineStyle: { color: line } },
      axisLabel: { color: text },
    },
    yAxis: {
      type: "value",
      splitLine: { lineStyle: { color: line } },
      axisLabel: { color: text },
    },
    series: [
      {
        name: "MCP 调用",
        type: "line",
        smooth: true,
        data: stats.value.call_series.map((p) => p.value),
        lineStyle: { color: cssVar("--brand"), width: 2 },
        itemStyle: { color: cssVar("--brand") },
        areaStyle: {
          color: {
            type: "linear", x: 0, y: 0, x2: 0, y2: 1,
            colorStops: [
              { offset: 0, color: "rgba(79,70,229,0.18)" },
              { offset: 1, color: "rgba(79,70,229,0)" },
            ],
          },
        },
      },
    ],
  };
});

const quickLinks = [
  { title: "邀请成员", desc: "为团队添加新成员并分配角色", to: "/app/users" },
  { title: "新建 API Key", desc: "为脚本与集成创建访问凭证", to: "/app/api-keys" },
  { title: "配置 MCP 工具", desc: "接入上游 MCP 服务并管理工具", to: "/app/mcp" },
];
</script>

<template>
  <div class="page">
    <h1 class="page-title">总览</h1>
    <p class="page-sub">{{ auth.tenant?.name }} · {{ auth.tenant?.plan_name }} 套餐</p>

    <div class="card-grid" style="margin-bottom: 16px">
      <StatCard
        v-for="c in cards"
        :key="c.label"
        :label="c.label"
        :value="c.value"
        :sub="c.sub"
        :icon="c.icon"
        :progress="c.progress"
        :danger="c.progress >= 95"
      />
    </div>

    <el-row :gutter="16">
      <el-col :xs="24" :lg="15">
        <el-card shadow="never">
          <template #header>近 7 天 MCP 调用量</template>
          <Chart v-if="chartOption" :option="chartOption" height="280px" />
        </el-card>
      </el-col>
      <el-col :xs="24" :lg="9">
        <el-card shadow="never" class="quick-card">
          <template #header>快捷操作</template>
          <div v-for="q in quickLinks" :key="q.title" class="quick-item" @click="$router.push(q.to)">
            <p class="quick-title">{{ q.title }}</p>
            <p class="quick-desc">{{ q.desc }}</p>
          </div>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<style scoped>
.page-title {
  margin: 0 0 4px;
  font-size: 18px;
  font-weight: 500;
}
.page-sub {
  margin: 0 0 16px;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}
.quick-item {
  padding: 12px 14px;
  border-radius: 10px;
  border: 1px solid var(--el-border-color-lighter);
  margin-bottom: 12px;
  cursor: pointer;
  transition: border-color 0.15s;
}
.quick-item:hover {
  border-color: var(--el-color-primary-light-5);
}
.quick-item:last-child {
  margin-bottom: 0;
}
.quick-title {
  margin: 0;
  font-size: 14px;
  font-weight: 500;
  color: var(--el-text-color-primary);
}
.quick-desc {
  margin: 4px 0 0;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
:deep(.el-card__header) {
  padding: 14px 20px;
  font-size: 14px;
  font-weight: 500;
  border-bottom: 1px solid var(--el-border-color-lighter);
}
</style>
