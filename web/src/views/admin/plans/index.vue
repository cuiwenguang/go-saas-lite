<script setup lang="ts">
import { Plus } from "@element-plus/icons-vue";
import { createPlan, listPlans, updatePlan } from "@/shared/api/platform";
import { PERM } from "@/shared/permission/perms";
import { useAuthStore } from "@/shared/auth/store";
import { fmtQuota } from "@/shared/utils/format";
import type { Plan, Quota } from "@/shared/api/types";
import type { FormInstance, FormRules } from "element-plus";

const auth = useAuthStore();
const plans = ref<Plan[]>([]);
const loading = ref(false);

async function load() {
  loading.value = true;
  try {
    plans.value = await listPlans();
  } finally {
    loading.value = false;
  }
}
onMounted(load);

type NumericQuotaKey = "max_users" | "storage_gb" | "api_qps" | "mcp_calls" | "mcp_tokens" | "mcp_sessions";
const quotaFields: { key: NumericQuotaKey; label: string; unit: string; step: number }[] = [
  { key: "max_users", label: "用户数上限", unit: "0 = 不限", step: 1 },
  { key: "storage_gb", label: "存储容量（GB）", unit: "0 = 不限", step: 5 },
  { key: "api_qps", label: "API QPS", unit: "次/秒，0 = 不限", step: 1 },
  { key: "mcp_calls", label: "MCP 调用量", unit: "次/周期，0 = 不限", step: 10000 },
  { key: "mcp_tokens", label: "MCP Tokens", unit: "0 = 不限", step: 100000 },
  { key: "mcp_sessions", label: "MCP 并发会话", unit: "0 = 不限", step: 1 },
];

function emptyQuota(): Quota {
  return { max_users: 0, storage_gb: 0, api_qps: 0, mcp_calls: 0, mcp_tokens: 0, mcp_sessions: 0, features: [] };
}

/* 新建 / 编辑共用弹窗 */
const dialog = ref(false);
const editing = ref<Plan | null>(null);
const saving = ref(false);
const formRef = ref<FormInstance>();
const form = reactive({ code: "", name: "", enabled: true, quota: emptyQuota() });
const rules: FormRules = {
  code: [
    { required: true, message: "请输入套餐编码", trigger: "blur" },
    { pattern: /^[a-z][a-z0-9_-]{1,31}$/, message: "小写字母开头，可含数字、- 和 _", trigger: "blur" },
  ],
  name: [{ required: true, message: "请输入套餐名称", trigger: "blur" }],
};

function openCreate() {
  editing.value = null;
  Object.assign(form, { code: "", name: "", enabled: true, quota: emptyQuota() });
  dialog.value = true;
  nextTick(() => formRef.value?.clearValidate());
}

function openEdit(plan: Plan) {
  editing.value = plan;
  Object.assign(form, {
    code: plan.code,
    name: plan.name,
    enabled: plan.enabled,
    quota: { ...plan.quota, features: [...plan.quota.features] },
  });
  dialog.value = true;
}

async function onSave() {
  const ok = await formRef.value?.validate().catch(() => false);
  if (!ok) return;
  saving.value = true;
  try {
    if (editing.value) {
      await updatePlan(editing.value.id, { name: form.name, quota: { ...form.quota }, enabled: form.enabled });
      ElMessage.success("套餐已更新");
    } else {
      await createPlan({ code: form.code, name: form.name, quota: { ...form.quota }, enabled: form.enabled });
      ElMessage.success("套餐已创建");
    }
    dialog.value = false;
    load();
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : "保存失败");
  } finally {
    saving.value = false;
  }
}

async function onToggle(plan: Plan) {
  try {
    await updatePlan(plan.id, { enabled: !plan.enabled });
    ElMessage.success(plan.enabled ? "已停用该套餐" : "已启用该套餐");
    load();
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : "操作失败");
  }
}
</script>

<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1 class="page-title">套餐管理</h1>
        <p class="page-desc">套餐 = 一组配额上限；值 0 表示不限。修改后对使用该套餐的租户最迟 30 秒生效。</p>
      </div>
      <el-button v-if="auth.can(PERM.planUpdate)" type="primary" :icon="Plus" @click="openCreate">新建套餐</el-button>
    </div>

    <div v-loading="loading" class="card-grid">
      <el-card v-for="p in plans" :key="p.id" shadow="never" :class="{ 'is-disabled': !p.enabled }">
        <div class="plan-head">
          <span class="plan-name">{{ p.name }}</span>
          <el-tag size="small" effect="plain" class="num">{{ p.code }}</el-tag>
          <div style="flex: 1" />
          <el-tag :type="p.enabled ? 'success' : 'info'" size="small">{{ p.enabled ? "启用" : "已停用" }}</el-tag>
        </div>
        <p class="plan-meta">引用租户 <span class="num">{{ p.tenant_count }}</span> 个</p>
        <ul class="plan-list">
          <li><span>用户数</span><span class="num">{{ fmtQuota(p.quota.max_users, "人") }}</span></li>
          <li><span>存储</span><span class="num">{{ fmtQuota(p.quota.storage_gb, "GB") }}</span></li>
          <li><span>API QPS</span><span class="num">{{ fmtQuota(p.quota.api_qps, "次/秒") }}</span></li>
          <li><span>MCP 调用</span><span class="num">{{ fmtQuota(p.quota.mcp_calls, "次") }}</span></li>
          <li><span>MCP Tokens</span><span class="num">{{ fmtQuota(p.quota.mcp_tokens, "") }}</span></li>
          <li><span>并发会话</span><span class="num">{{ fmtQuota(p.quota.mcp_sessions, "个") }}</span></li>
          <li v-if="p.quota.features.length"><span>特性</span><span>{{ p.quota.features.join(" · ") }}</span></li>
        </ul>
        <div v-if="auth.can(PERM.planUpdate)" class="plan-actions">
          <el-button text type="primary" size="small" @click="openEdit(p)">编辑</el-button>
          <el-button text size="small" @click="onToggle(p)">{{ p.enabled ? "停用" : "启用" }}</el-button>
        </div>
      </el-card>
      <el-empty v-if="!loading && !plans.length" description="还没有套餐" />
    </div>

    <el-dialog
      v-model="dialog"
      :title="editing ? `编辑套餐 · ${editing.name}` : '新建套餐'"
      width="560px"
      :close-on-click-modal="false"
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
        <el-row :gutter="12">
          <el-col :span="12">
            <el-form-item label="套餐编码" prop="code">
              <el-input v-model="form.code" :disabled="!!editing" placeholder="如：pro" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="套餐名称" prop="name">
              <el-input v-model="form.name" placeholder="如：专业版" />
            </el-form-item>
          </el-col>
        </el-row>
        <el-row :gutter="12">
          <el-col v-for="f in quotaFields" :key="f.key" :span="12">
            <el-form-item :label="f.label">
              <el-input-number v-model="form.quota[f.key]" :min="0" :step="f.step" style="width: 100%" />
              <div class="field-hint">{{ f.unit }}</div>
            </el-form-item>
          </el-col>
        </el-row>
        <el-form-item label="特性标签">
          <el-select
            v-model="form.quota.features"
            multiple
            filterable
            allow-create
            default-first-option
            placeholder="如：sso / audit"
            style="width: 100%"
          />
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialog = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="onSave">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.page-title {
  margin: 0 0 8px;
  font-size: 18px;
  font-weight: 500;
}
.page-desc {
  margin: 0;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}
.page-head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 20px;
}
.plan-meta {
  margin: 10px 0 4px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.plan-actions {
  display: flex;
  justify-content: flex-end;
  gap: 4px;
  margin-top: 8px;
}
.is-disabled {
  opacity: 0.6;
}
.plan-head {
  display: flex;
  align-items: center;
  gap: 8px;
}
.plan-name {
  font-size: 16px;
  font-weight: 500;
}
.plan-price {
  margin: 10px 0 14px;
  font-size: 24px;
  font-weight: 500;
  color: var(--el-color-primary);
}
.plan-unit {
  font-size: 13px;
  color: var(--el-text-color-secondary);
  font-weight: 400;
}
.plan-list {
  list-style: none;
  margin: 0;
  padding: 0;
}
.plan-list li {
  display: flex;
  justify-content: space-between;
  padding: 7px 0;
  font-size: 13px;
  border-bottom: 1px solid var(--el-border-color-lighter);
}
.plan-list li:last-child {
  border-bottom: none;
}
.field-hint {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  line-height: 1.4;
  margin-top: 4px;
}
</style>
