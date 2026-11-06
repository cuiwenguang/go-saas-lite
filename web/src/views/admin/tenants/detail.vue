<script setup lang="ts">
import { ArrowLeft, Key, RefreshRight } from "@element-plus/icons-vue";
import {
  getTenant, getTenantUsage, listTenantUsers, resetTenantAdminPassword, retryProvision, setTenantStatus, updateTenant,
} from "@/shared/api/platform";
import { PERM } from "@/shared/permission/perms";
import { useAuthStore } from "@/shared/auth/store";
import { cssVar, fmtDateTime, fmtNumber, fmtQuota, pct } from "@/shared/utils/format";
import Chart from "@/shared/components/Chart.vue";
import type { TenantDetail, TenantStatus, TenantUsage, TenantUser } from "@/shared/api/types";
import type { EChartsCoreOption } from "echarts/core";

const auth = useAuthStore();
const route = useRoute();
const router = useRouter();

const id = String(route.params.id);
const detail = ref<TenantDetail | null>(null);
const users = ref<TenantUser[]>([]);
const usage = ref<TenantUsage | null>(null);
const loading = ref(false);
const tab = ref("basic");

const statusText: Record<TenantStatus, string> = {
  pending: "待开通", active: "启用", suspended: "已停用", expired: "已到期", archived: "已归档",
};
const statusTag: Record<TenantStatus, "success" | "warning" | "info" | "danger"> = {
  pending: "warning", active: "success", suspended: "warning", expired: "danger", archived: "info",
};

/** 开通步骤的中文说明；未知步骤直接显示原始值 */
const provisionStepText: Record<string, string> = {
  tenant_created: "写入租户记录",
  provision_retried: "重新执行开通",
  tenant_bootstrapped: "初始化角色与管理员",
  done: "开通完成",
  failed: "开通失败",
};

const provisionStatusText: Record<string, string> = {
  pending: "待开通", running: "开通中", succeeded: "开通成功", failed: "开通失败",
};

async function load() {
  loading.value = true;
  try {
    const [tenant, tenantUsers, tenantUsage] = await Promise.all([
      getTenant(id),
      listTenantUsers(id),
      getTenantUsage(id),
    ]);
    detail.value = tenant;
    users.value = tenantUsers.items;
    usage.value = tenantUsage;
  } finally {
    loading.value = false;
  }
}
onMounted(load);

/* ---------- 配额与用量 ---------- */

const quotaRows = computed(() => {
  const data = usage.value;
  if (!data) return [];
  return [
    { label: "用户数", used: data.usage.users, total: data.max_users },
    { label: "MCP 调用（近 30 天）", used: data.usage.mcp_calls, total: data.mcp_calls_cap },
    { label: "MCP Tokens（近 30 天）", used: data.usage.mcp_tokens, total: data.mcp_tokens_cap },
  ];
});

const chartOption = computed<EChartsCoreOption | null>(() => {
  const data = usage.value;
  if (!data || !data.series.length) return null;
  const text = cssVar("--el-text-color-secondary");
  const line = cssVar("--el-border-color-light");
  return {
    grid: { left: 50, right: 16, top: 24, bottom: 28 },
    tooltip: { trigger: "axis" },
    xAxis: {
      type: "category",
      data: data.series.map((point) => point.date),
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
        type: "line",
        smooth: true,
        data: data.series.map((point) => point.value),
        lineStyle: { color: cssVar("--brand"), width: 2 },
        itemStyle: { color: cssVar("--brand") },
        areaStyle: { color: "rgba(79,70,229,0.08)" },
      },
    ],
  };
});

async function onToggleStatus() {
  const d = detail.value;
  if (!d) return;
  const next: TenantStatus = d.status === "active" ? "suspended" : "active";
  const action = next === "active" ? "启用" : "停用";
  await ElMessageBox.confirm(`确定${action}「${d.name}」？`, `${action}租户`, { type: "warning" }).catch(() => Promise.reject());
  await setTenantStatus(d.id, next);
  d.status = next;
  ElMessage.success(`已${action}`);
}

/* ---------- 续费（改 expire_at） ---------- */

const renewDialog = ref(false);
const renewAt = ref("");
const saving = ref(false);

function openRenew() {
  const current = detail.value?.expire_at ? new Date(detail.value.expire_at) : new Date();
  const base = current > new Date() ? current : new Date();
  base.setFullYear(base.getFullYear() + 1);
  renewAt.value = base.toISOString().slice(0, 10);
  renewDialog.value = true;
}

async function onRenew() {
  if (!renewAt.value) return;
  saving.value = true;
  try {
    detail.value = await updateTenant(id, { expire_at: new Date(`${renewAt.value}T00:00:00Z`).toISOString() });
    renewDialog.value = false;
    ElMessage.success("已续费");
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : "续费失败");
  } finally {
    saving.value = false;
  }
}

/* ---------- 重试开通 / 重置管理员密码 ---------- */

const credential = ref<{ title: string; password: string } | null>(null);

async function onRetry() {
  await ElMessageBox.confirm("将重新执行开通流水线，并重置管理员初始密码，确定继续？", "重试开通", {
    type: "warning",
    confirmButtonText: "重试",
  }).catch(() => Promise.reject());
  const r = await retryProvision(id);
  credential.value = { title: "已重试开通", password: r.initial_password };
  await load();
}

async function onResetPassword() {
  await ElMessageBox.confirm("将生成新的管理员初始密码，旧密码与登录态立即失效，确定继续？", "重置管理员密码", {
    type: "warning",
    confirmButtonText: "重置",
  }).catch(() => Promise.reject());
  const r = await resetTenantAdminPassword(id);
  credential.value = { title: "管理员密码已重置", password: r.initial_password };
}

async function copyPassword() {
  if (!credential.value) return;
  await navigator.clipboard.writeText(credential.value.password).catch(() => undefined);
  ElMessage.success("已复制到剪贴板");
}
</script>

<template>
  <div v-loading="loading" class="page">
    <div class="head">
      <el-button :icon="ArrowLeft" text @click="router.back()">返回</el-button>
      <h1 class="page-title">
        {{ detail?.name || "租户详情" }}
        <el-tag v-if="detail" :type="statusTag[detail.status]" size="small" class="status-pill">{{ statusText[detail.status] }}</el-tag>
      </h1>
      <div style="flex: 1" />
      <template v-if="detail">
        <el-button
          v-if="['active', 'suspended'].includes(detail.status) && auth.can(PERM.tenantUpdate)"
          :type="detail.status === 'active' ? 'warning' : 'success'"
          @click="onToggleStatus"
        >{{ detail.status === "active" ? "停用租户" : "启用租户" }}</el-button>
        <el-button v-if="auth.can(PERM.tenantUpdate)" @click="openRenew">续费</el-button>
        <el-button v-if="auth.can(PERM.tenantUpdate)" :icon="Key" @click="onResetPassword">重置管理员密码</el-button>
      </template>
    </div>

    <el-card v-if="detail" shadow="never">
      <el-tabs v-model="tab">
        <el-tab-pane label="基本信息" name="basic">
          <el-descriptions :column="2" border>
            <el-descriptions-item label="租户 ID"><span class="num">{{ detail.id }}</span></el-descriptions-item>
            <el-descriptions-item label="租户标识"><span class="num">{{ detail.code }}</span></el-descriptions-item>
            <el-descriptions-item label="访问域名">
              {{ detail.domain || (detail.subdomain ? `${detail.subdomain}.example.com` : "未配置") }}
            </el-descriptions-item>
            <el-descriptions-item label="套餐">
              <el-tag size="small" effect="plain">{{ detail.plan_name || detail.plan_code }}</el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="到期时间">
              {{ detail.expire_at ? fmtDateTime(detail.expire_at) : "长期有效" }}
            </el-descriptions-item>
            <el-descriptions-item label="管理员">{{ detail.admin_email || "未记录" }}</el-descriptions-item>
            <el-descriptions-item label="时区 / 语言">{{ detail.timezone }} / {{ detail.locale }}</el-descriptions-item>
            <el-descriptions-item label="联系人">{{ detail.contact_name || "-" }}</el-descriptions-item>
            <el-descriptions-item label="联系电话">{{ detail.contact_phone || "-" }}</el-descriptions-item>
            <el-descriptions-item label="联系邮箱">{{ detail.contact_email || "-" }}</el-descriptions-item>
            <el-descriptions-item label="创建时间"><span class="num">{{ fmtDateTime(detail.created_at) }}</span></el-descriptions-item>
            <el-descriptions-item label="更新时间"><span class="num">{{ fmtDateTime(detail.updated_at) }}</span></el-descriptions-item>
          </el-descriptions>
        </el-tab-pane>

        <el-tab-pane label="开通进度" name="provision">
          <div class="provision-head">
            <el-tag
              :type="detail.provision_status === 'succeeded' ? 'success' : detail.provision_status === 'failed' ? 'danger' : 'warning'"
              size="small"
            >{{ provisionStatusText[detail.provision_status] || detail.provision_status }}</el-tag>
            <span class="t-sub">当前步骤：{{ provisionStepText[detail.provision_step || ""] || detail.provision_step || "-" }}</span>
            <div style="flex: 1" />
            <el-button
              v-if="detail.provision_status !== 'succeeded' && auth.can(PERM.tenantUpdate)"
              type="primary"
              :icon="RefreshRight"
              @click="onRetry"
            >重试开通</el-button>
          </div>

          <el-timeline v-if="detail.provision_log.length" class="timeline">
            <el-timeline-item
              v-for="(e, i) in detail.provision_log"
              :key="i"
              :timestamp="fmtDateTime(e.at)"
              :type="e.ok ? 'success' : 'danger'"
            >
              {{ provisionStepText[e.step] || e.step }}
              <span v-if="e.error" class="t-sub"> · {{ e.error }}</span>
            </el-timeline-item>
          </el-timeline>
          <el-empty v-else description="暂无开通记录" :image-size="72" />
        </el-tab-pane>

        <el-tab-pane :label="`用户与角色（${users.length}）`" name="users">
          <el-table :data="users" style="width: 100%">
            <el-table-column prop="email" label="邮箱" min-width="200" />
            <el-table-column prop="display_name" label="姓名" width="130" />
            <el-table-column label="角色" width="170">
              <template #default="{ row }">
                <el-tag v-for="r in row.roles" :key="r" size="small" effect="plain" class="role-tag">{{ r }}</el-tag>
                <span v-if="!row.roles.length" class="t-sub">-</span>
              </template>
            </el-table-column>
            <el-table-column label="状态" width="100">
              <template #default="{ row }">
                <el-tag :type="row.status === 'active' ? 'success' : 'info'" size="small">
                  {{ row.status === "active" ? "启用" : "停用" }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="改密" width="100">
              <template #default="{ row }">
                <el-tag v-if="row.must_change_password" type="warning" size="small">待改密</el-tag>
                <span v-else class="t-sub">-</span>
              </template>
            </el-table-column>
            <el-table-column label="创建时间" width="180">
              <template #default="{ row }"><span class="num">{{ fmtDateTime(row.created_at) }}</span></template>
            </el-table-column>
            <template #empty>
              <el-empty description="暂无用户" :image-size="72" />
            </template>
          </el-table>
        </el-tab-pane>

        <el-tab-pane label="配额与用量" name="usage">
          <template v-if="usage">
            <el-descriptions :column="4" border class="usage-desc">
              <el-descriptions-item label="当前套餐">
                <el-tag size="small" effect="plain">{{ usage.plan_code }}</el-tag>
              </el-descriptions-item>
              <el-descriptions-item label="用户数上限">{{ fmtQuota(usage.max_users, "人") }}</el-descriptions-item>
              <el-descriptions-item label="MCP 调用上限">{{ fmtQuota(usage.mcp_calls_cap, "次") }}</el-descriptions-item>
              <el-descriptions-item label="API QPS">{{ fmtQuota(usage.api_qps, "次/秒") }}</el-descriptions-item>
            </el-descriptions>

            <div class="quota-grid">
              <div v-for="q in quotaRows" :key="q.label" class="quota-row">
                <div class="quota-head">
                  <span>{{ q.label }}</span>
                  <span class="num quota-nums">{{ fmtNumber(q.used) }} / {{ fmtQuota(q.total, "") }}</span>
                </div>
                <el-progress
                  :percentage="pct(q.used, q.total)"
                  :status="pct(q.used, q.total) >= 90 ? 'warning' : undefined"
                  :stroke-width="6"
                  :show-text="false"
                />
              </div>
            </div>

            <p class="section-title">近 7 天 MCP 调用</p>
            <!-- 图表在隐藏 tab 里初始化会拿到 0 宽高，切到本 tab 后再挂载 -->
            <Chart v-if="chartOption && tab === 'usage'" :option="chartOption" height="260px" />
            <el-empty v-else description="暂无调用数据" :image-size="64" />
          </template>
          <el-empty v-else description="暂无用量数据" :image-size="72" />
        </el-tab-pane>
      </el-tabs>
    </el-card>

    <!-- 续费：改 expire_at -->
    <el-dialog v-model="renewDialog" title="续费" width="380px" :close-on-click-modal="false">
      <el-form label-position="top">
        <el-form-item label="新的到期时间">
          <el-date-picker v-model="renewAt" type="date" value-format="YYYY-MM-DD" style="width: 100%" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="renewDialog = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="onRenew">确定</el-button>
      </template>
    </el-dialog>

    <!-- 一次性密码展示 -->
    <el-dialog
      :model-value="credential !== null"
      :title="credential?.title || '凭据'"
      width="440px"
      :close-on-click-modal="false"
      @update:model-value="(v: boolean) => { if (!v) credential = null }"
    >
      <el-alert type="warning" :closable="false" show-icon>
        <p>初始密码（<b>仅显示这一次</b>，请立即复制保存）：</p>
      </el-alert>
      <div class="password-box num">{{ credential?.password }}</div>
      <template #footer>
        <el-button @click="copyPassword">复制密码</el-button>
        <el-button type="primary" @click="credential = null">我已保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.head {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 16px;
}
.page-title {
  margin: 0;
  font-size: 18px;
  font-weight: 500;
  display: flex;
  align-items: center;
  gap: 10px;
}
.provision-head {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 20px;
}
.timeline {
  padding-left: 4px;
}
.role-tag + .role-tag {
  margin-left: 6px;
}
.password-box {
  margin-top: 12px;
  padding: 12px 16px;
  border-radius: 8px;
  background: var(--el-fill-color-light);
  font-size: 15px;
  letter-spacing: 0.5px;
  word-break: break-all;
}
.usage-desc {
  margin-bottom: 20px;
}
.quota-grid {
  display: grid;
  gap: 16px;
  margin-bottom: 8px;
}
.quota-head {
  display: flex;
  justify-content: space-between;
  font-size: 13px;
  margin-bottom: 6px;
}
.quota-nums {
  color: var(--el-text-color-secondary);
}
.section-title {
  margin: 20px 0 8px;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}
</style>
