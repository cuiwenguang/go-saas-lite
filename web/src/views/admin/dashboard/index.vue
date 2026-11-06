<script setup lang="ts">
import { OfficeBuilding, Odometer, TrendCharts } from "@element-plus/icons-vue";
import { platformStats, listTenants, usageTop } from "@/shared/api/platform";
import { PERM } from "@/shared/permission/perms";
import { useAuthStore } from "@/shared/auth/store";
import { cssVar, fmtDateTime, fmtNumber } from "@/shared/utils/format";
import StatCard from "@/shared/components/StatCard.vue";
import Chart from "@/shared/components/Chart.vue";
import type { PlatformStats, Paged, TenantRow, UsageTopRow } from "@/shared/api/types";
import type { EChartsCoreOption } from "echarts/core";

const auth = useAuthStore();
const stats = ref<PlatformStats | null>(null);
const recent = ref<TenantRow[]>([]);
const topUsage = ref<UsageTopRow[]>([]);

onMounted(async () => {
  if (auth.can(PERM.statsRead)) {
    stats.value = await platformStats();
    topUsage.value = await usageTop({ metric: "mcp_calls", limit: 5 });
  }
  if (auth.can(PERM.tenantRead)) {
    const p: Paged<TenantRow> = await listTenants({ page: 1, size: 5 });
    recent.value = p.items;
  }
});

const chartOption = computed<EChartsCoreOption | null>(() => {
  if (!stats.value) return null;
  const text = cssVar("--el-text-color-secondary");
  const line = cssVar("--el-border-color-light");
  return {
    grid: { left: 40, right: 16, top: 24, bottom: 28 },
    tooltip: { trigger: "axis" },
    xAxis: {
      type: "category",
      data: stats.value.new_series.map((p) => p.date),
      axisLine: { lineStyle: { color: line } },
      axisLabel: { color: text },
    },
    yAxis: {
      type: "value",
      minInterval: 1,
      splitLine: { lineStyle: { color: line } },
      axisLabel: { color: text },
    },
    series: [
      {
        type: "line",
        smooth: true,
        data: stats.value.new_series.map((p) => p.value),
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

const router = useRouter();
function goTenant(id: string) {
  router.push({ name: "tenant-detail", params: { id } });
}
</script>

<template>
  <div class="page">
    <h1 class="page-title">总览</h1>

    <div class="card-grid" style="margin-bottom: 16px">
      <StatCard v-if="stats" label="租户总数" :value="stats.tenant_total" :icon="OfficeBuilding" :sub="`活跃 ${stats.tenant_active}`" />
      <StatCard v-if="stats" label="活跃租户" :value="stats.tenant_active" :icon="Odometer" :sub="`本周新增 ${stats.new_this_week}`" />
      <StatCard v-if="stats" label="本周新增" :value="stats.new_this_week" :icon="TrendCharts" />
      <StatCard
        v-if="stats"
        label="30 天内到期"
        :value="stats.expiring_soon"
        :icon="TrendCharts"
        sub="需要安排续费"
      />
    </div>

    <el-alert
      v-if="stats && stats.provision_failed > 0"
      type="warning"
      :closable="false"
      show-icon
      class="pending-alert"
      :title="`有 ${stats.provision_failed} 个租户开通失败，需要重试`"
    >
      <el-button text type="primary" size="small" @click="router.push('/admin/tenants')">去租户管理查看</el-button>
    </el-alert>

    <el-row :gutter="16">
      <el-col :xs="24" :lg="14">
        <el-card shadow="never">
          <template #header>近 7 天新增租户</template>
          <Chart v-if="chartOption" :option="chartOption" height="260px" />
        </el-card>
      </el-col>
      <el-col :xs="24" :lg="10">
        <el-card shadow="never" class="recent-card">
          <template #header>MCP 调用 Top5</template>
          <div v-if="topUsage.length" class="usage-list">
            <div v-for="(item, index) in topUsage" :key="item.tenant_id" class="usage-item" @click="goTenant(item.tenant_id)">
              <span class="usage-rank" :class="`rank-${index + 1}`">{{ index + 1 }}</span>
              <span class="usage-name">{{ item.name }}</span>
              <span class="num usage-value">{{ fmtNumber(item.value) }}</span>
            </div>
          </div>
          <el-empty v-else description="暂无用量数据" :image-size="64" />
        </el-card>
      </el-col>
    </el-row>

    <el-card shadow="never" class="recent-card" style="margin-top: 16px">
          <template #header>最新租户</template>
          <el-table :data="recent" :show-header="false" style="width: 100%">
            <el-table-column label="租户">
              <template #default="{ row }">
                <p class="t-name">{{ row.name }}</p>
                <p class="t-sub">{{ row.subdomain ? `${row.subdomain}.example.com` : row.code }}</p>
              </template>
            </el-table-column>
            <el-table-column label="套餐" width="100">
              <template #default="{ row }">
                <el-tag size="small" effect="plain">{{ row.plan_name || row.plan_code }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="创建时间" width="100">
              <template #default="{ row }">
                <span class="t-sub num">{{ fmtDateTime(row.created_at).slice(0, 10) }}</span>
              </template>
            </el-table-column>
            <el-table-column width="60" fixed="right">
              <template #default="{ row }">
                <el-button text type="primary" size="small" @click="goTenant(row.id)">详情</el-button>
              </template>
            </el-table-column>
          </el-table>
    </el-card>
  </div>
</template>

<style scoped>
.page-title {
  margin: 0 0 16px;
  font-size: 18px;
  font-weight: 500;
}
.t-name {
  margin: 0;
  font-size: 13px;
  color: var(--el-text-color-primary);
}
.t-sub {
  margin: 2px 0 0;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
:deep(.el-card__header) {
  padding: 14px 20px;
  font-size: 14px;
  font-weight: 500;
  border-bottom: 1px solid var(--el-border-color-lighter);
}
.pending-alert {
  margin-bottom: 16px;
}
.usage-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.usage-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px;
  border-radius: 8px;
  cursor: pointer;
  transition: background 0.15s;
}
.usage-item:hover {
  background: var(--el-fill-color-light);
}
.usage-rank {
  width: 20px;
  height: 20px;
  border-radius: 6px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  background: var(--el-fill-color);
}
.rank-1 {
  color: #fff;
  background: var(--brand-strong);
}
.usage-name {
  flex: 1;
  font-size: 13px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.usage-value {
  font-size: 13px;
  color: var(--el-text-color-secondary);
}
</style>
