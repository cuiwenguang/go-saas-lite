<script setup lang="ts">
import { Refresh, Search } from "@element-plus/icons-vue";
import { listAuditLogs } from "@/shared/api/platform";
import { fmtDateTime } from "@/shared/utils/format";
import type { AuditLogRow } from "@/shared/api/types";

const rows = ref<AuditLogRow[]>([]);
const total = ref(0);
const loading = ref(false);
const query = reactive({
  page: 1,
  size: 20,
  category: "",
  action: "",
  actor: "",
  level: "",
  range: [] as string[],
});

/** 一级筛选：分类 chips */
const categories = [
  { value: "", label: "全部" },
  { value: "tenant", label: "租户" },
  { value: "plan", label: "套餐" },
  { value: "auth", label: "登录" },
  { value: "security", label: "安全" },
  { value: "config", label: "配置" },
  { value: "system", label: "系统" },
];

/** 二级筛选：动作随分类联动（与后端 audit 包的动作常量同源） */
const actionsByCategory: Record<string, { value: string; label: string }[]> = {
  tenant: [
    { value: "tenant.create", label: "开通租户" },
    { value: "tenant.update", label: "修改租户" },
    { value: "tenant.renew", label: "续费" },
    { value: "tenant.update_status", label: "变更状态" },
    { value: "tenant.retry_provision", label: "重试开通" },
    { value: "tenant.reset_admin_password", label: "重置管理员密码" },
  ],
  plan: [
    { value: "plan.create", label: "新建套餐" },
    { value: "plan.update", label: "修改套餐" },
    { value: "plan.toggle", label: "启停套餐" },
  ],
  auth: [
    { value: "platform.login", label: "登录" },
    { value: "platform.login_failed", label: "登录失败" },
    { value: "platform.password_change", label: "修改密码" },
  ],
  security: [{ value: "tenant.cross_tenant_read", label: "跨租户查看" }],
  config: [{ value: "config.update", label: "修改配置" }],
  system: [{ value: "tenant.auto_expire", label: "自动到期" }],
};

const actionLabels = computed(() => {
  const map: Record<string, string> = {};
  for (const list of Object.values(actionsByCategory)) {
    for (const item of list) map[item.value] = item.label;
  }
  return map;
});
const actionOptions = computed(() => actionsByCategory[query.category] || []);

const categoryLabels: Record<string, string> = {
  tenant: "租户",
  plan: "套餐",
  auth: "登录",
  security: "安全",
  config: "配置",
  system: "系统",
};

async function load() {
  loading.value = true;
  try {
    const page = await listAuditLogs({
      page: query.page,
      size: query.size,
      category: query.category || undefined,
      action: query.action || undefined,
      level: query.level || undefined,
      actor: query.actor.trim() || undefined,
      from: query.range[0] ? new Date(`${query.range[0]}T00:00:00`).toISOString() : undefined,
      to: query.range[1] ? new Date(`${query.range[1]}T23:59:59`).toISOString() : undefined,
    });
    rows.value = page.items;
    total.value = page.total;
  } finally {
    loading.value = false;
  }
}
onMounted(load);

function pickCategory(value: string) {
  if (query.category === value) return;
  query.category = value;
  query.action = "";
  query.page = 1;
  load();
}

function onSearch() {
  query.page = 1;
  load();
}

/** 只看风险操作 */
const onlyWarn = computed({
  get: () => query.level === "warn",
  set: (value: boolean) => {
    query.level = value ? "warn" : "";
    query.page = 1;
    load();
  },
});

const detail = ref<AuditLogRow | null>(null);
const detailOpen = computed({
  get: () => detail.value !== null,
  set: (value: boolean) => { if (!value) detail.value = null; },
});

function pretty(value?: Record<string, unknown>) {
  return value ? JSON.stringify(value, null, 2) : "";
}
</script>

<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1 class="page-title">审计日志</h1>
        <p class="page-sub">平台侧操作留痕：谁、何时、对什么做了什么。日志只增不改，按分类与风险级别筛选。</p>
      </div>
    </div>

    <div class="seg-bar">
      <button
        v-for="item in categories"
        :key="item.value"
        type="button"
        class="seg"
        :class="{ active: query.category === item.value }"
        @click="pickCategory(item.value)"
      >{{ item.label }}</button>
    </div>

    <el-card shadow="never" class="table-card">
      <div class="card-toolbar">
        <el-date-picker
          v-model="query.range"
          type="daterange"
          value-format="YYYY-MM-DD"
          range-separator="→"
          start-placeholder="开始日期"
          end-placeholder="结束日期"
          class="range-picker"
          @change="onSearch"
        />
        <el-select
          v-model="query.action"
          clearable
          placeholder="全部动作"
          class="action-select"
          :disabled="!query.category"
          @change="onSearch"
        >
          <el-option v-for="item in actionOptions" :key="item.value" :value="item.value" :label="item.label" />
        </el-select>
        <el-input
          v-model="query.actor"
          placeholder="操作人邮箱 / ID"
          :prefix-icon="Search"
          clearable
          class="actor-input"
          @clear="onSearch"
          @keyup.enter="onSearch"
        />
        <el-switch v-model="onlyWarn" active-text="只看风险操作" />
        <div class="toolbar-spacer" />
        <el-button text :icon="Refresh" title="刷新" @click="load" />
      </div>

      <el-table v-loading="loading" :data="rows" style="width: 100%" row-class-name="row-clickable" @row-click="detail = $event">
        <el-table-column label="时间" width="150">
          <template #default="{ row }"><span class="num t-sub">{{ fmtDateTime(row.created_at) }}</span></template>
        </el-table-column>
        <el-table-column label="分类" width="90">
          <template #default="{ row }">
            <el-tag size="small" effect="plain">{{ categoryLabels[row.category] || row.category }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="动作" width="140">
          <template #default="{ row }">
            <span class="action">{{ actionLabels[row.action] || row.action }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="summary" label="摘要" min-width="240" show-overflow-tooltip />
        <el-table-column label="操作人" width="200">
          <template #default="{ row }">
            <span v-if="row.actor_type === 'system'" class="t-sub">系统任务</span>
            <span v-else>{{ row.actor_email || "-" }}</span>
          </template>
        </el-table-column>
        <el-table-column label="级别" width="90">
          <template #default="{ row }">
            <span class="status-pill" :class="row.level === 'warn' ? 'pill-warn' : 'pill-info'">
              {{ row.level === "warn" ? "风险" : "常规" }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="结果" width="90">
          <template #default="{ row }">
            <span class="t-sub">{{ row.result === "failed" ? "失败" : "成功" }}</span>
          </template>
        </el-table-column>
        <template #empty>
          <div class="empty">
            <p class="empty-title">没有匹配的审计记录</p>
            <p class="empty-sub">换个分类、时间范围或操作人试试</p>
          </div>
        </template>
      </el-table>

      <div class="table-foot">
        <span class="foot-total">共 {{ total }} 条</span>
        <el-pagination
          v-model:current-page="query.page"
          v-model:page-size="query.size"
          :total="total"
          :page-sizes="[20, 50, 100]"
          layout="sizes, prev, pager, next"
          @current-change="load"
          @size-change="onSearch"
        />
      </div>
    </el-card>

    <el-drawer v-model="detailOpen" title="审计详情" size="520px">
      <template v-if="detail">
        <el-descriptions :column="1" border>
          <el-descriptions-item label="时间"><span class="num">{{ fmtDateTime(detail.created_at) }}</span></el-descriptions-item>
          <el-descriptions-item label="动作">{{ actionLabels[detail.action] || detail.action }}</el-descriptions-item>
          <el-descriptions-item label="摘要">{{ detail.summary }}</el-descriptions-item>
          <el-descriptions-item label="操作人">
            {{ detail.actor_type === "system" ? "系统任务" : detail.actor_email || detail.actor_type }}
          </el-descriptions-item>
          <el-descriptions-item label="对象">
            <span class="num">{{ detail.object_type }} / {{ detail.object_id || "-" }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="来源">
            <span class="num">{{ detail.ip || "-" }} · {{ detail.request_id || "-" }}</span>
          </el-descriptions-item>
        </el-descriptions>

        <template v-if="detail.before || detail.after">
          <p class="drawer-title">变更前</p>
          <pre class="json">{{ pretty(detail.before) || "—" }}</pre>
          <p class="drawer-title">变更后</p>
          <pre class="json">{{ pretty(detail.after) || "—" }}</pre>
        </template>
      </template>
    </el-drawer>
  </div>
</template>

<style scoped>
.page-head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  margin-bottom: 14px;
}
.page-title {
  margin: 0;
  font-size: 20px;
  font-weight: 500;
  letter-spacing: -0.2px;
}
.page-sub {
  margin: 4px 0 0;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.seg-bar {
  display: flex;
  gap: 6px;
  margin-bottom: 12px;
}
.seg {
  border: none;
  background: transparent;
  font-family: inherit;
  font-size: 13px;
  color: var(--el-text-color-regular);
  padding: 6px 12px;
  border-radius: 999px;
  cursor: pointer;
  transition: background 0.15s, color 0.15s;
}
.seg:hover {
  background: var(--el-fill-color-light);
}
.seg.active {
  background: var(--el-color-primary-light-9);
  color: var(--el-color-primary);
}

.table-card {
  padding: 8px 4px 4px;
}
.table-card :deep(.el-card__body) {
  padding: 0;
}
.card-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 16px 12px;
}
.range-picker {
  width: 260px;
}
.action-select {
  width: 160px;
}
.actor-input {
  width: 200px;
}
.toolbar-spacer {
  flex: 1;
}

.action {
  font-size: 13px;
}

/* 级别软底色圆点 pill（与 CONVENTIONS-FRONTEND §8 一致） */
.status-pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
}
.status-pill::before {
  content: "";
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--el-color-success);
}
.pill-warn::before {
  background: var(--el-color-warning);
}

.drawer-title {
  margin: 20px 0 8px;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}
.json {
  margin: 0;
  padding: 12px;
  border-radius: 8px;
  background: var(--el-fill-color-light);
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
