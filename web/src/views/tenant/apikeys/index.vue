<script setup lang="ts">
import { Key, Plus } from "@element-plus/icons-vue";
import { createApiKey, listApiKeys, revokeApiKey } from "@/shared/api/tenant";
import { PERM } from "@/shared/permission/perms";
import { useAuthStore } from "@/shared/auth/store";
import { fmtDateTime } from "@/shared/utils/format";
import type { ApiKeyRow } from "@/shared/api/types";
import type { FormInstance, FormRules } from "element-plus";

const auth = useAuthStore();

const rows = ref<ApiKeyRow[]>([]);
const loading = ref(false);

async function load() {
  loading.value = true;
  try {
    const r = await listApiKeys();
    rows.value = r.items;
  } finally {
    loading.value = false;
  }
}
onMounted(load);

async function onRevoke(row: ApiKeyRow) {
  await ElMessageBox.confirm(
    `吊销「${row.name}」后，使用该 Key 的所有调用立即失败（401），确定？`,
    "吊销 API Key",
    { type: "warning", confirmButtonText: "吊销", confirmButtonClass: "el-button--danger" },
  ).catch(() => Promise.reject());
  await revokeApiKey(row.id);
  ElMessage.success("已吊销");
  load();
}

/* ---------- 新建（secret 仅显示一次） ---------- */

const dialog = ref(false);
const submitLoading = ref(false);
const formRef = ref<FormInstance>();
const form = reactive({ name: "" });
const rules: FormRules = { name: [{ required: true, message: "请输入名称", trigger: "blur" }] };

const created = ref<{ name: string; secret: string } | null>(null);
const createdDialog = computed({
  get: () => created.value !== null,
  set: (v: boolean) => { if (!v) created.value = null; },
});

async function onCreate() {
  const ok = await formRef.value?.validate().catch(() => false);
  if (!ok) return;
  submitLoading.value = true;
  try {
    const r = await createApiKey(form.name);
    dialog.value = false;
    created.value = { name: r.key.name, secret: r.plain };
    form.name = "";
    load();
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : "创建失败");
  } finally {
    submitLoading.value = false;
  }
}

async function copySecret() {
  if (!created.value) return;
  await navigator.clipboard.writeText(created.value.secret).catch(() => undefined);
  ElMessage.success("已复制到剪贴板");
}
</script>

<template>
  <div class="page">
    <h1 class="page-title">API Key</h1>
    <p class="page-desc">
      供无法走 OAuth 的脚本与系统集成使用。Secret 只在创建时显示一次，服务端仅存哈希。
    </p>

    <div class="toolbar">
      <div class="toolbar-spacer" />
      <el-button v-if="auth.can(PERM.apiKeyCreate)" v-permission="PERM.apiKeyCreate" type="primary" :icon="Plus" @click="dialog = true">
        新建 Key
      </el-button>
    </div>

    <el-card shadow="never">
      <el-table v-loading="loading" :data="rows" style="width: 100%">
        <el-table-column label="名称" min-width="160">
          <template #default="{ row }">
            <div class="k-cell">
              <el-icon class="k-icon"><Key /></el-icon>
              <span class="k-name">{{ row.name }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="Key 前缀" width="200">
          <template #default="{ row }">
            <code class="k-prefix num">{{ row.key_prefix }}…</code>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="row.revoked_at ? 'info' : 'success'" size="small" class="status-pill">
              {{ row.revoked_at ? "已吊销" : "生效中" }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="创建时间" width="170">
          <template #default="{ row }"><span class="num k-sub">{{ fmtDateTime(row.created_at) }}</span></template>
        </el-table-column>
        <el-table-column label="最近使用" width="170">
          <template #default="{ row }"><span class="num k-sub">{{ row.last_used_at ? fmtDateTime(row.last_used_at) : "未使用" }}</span></template>
        </el-table-column>
        <el-table-column label="操作" width="90" fixed="right">
          <template #default="{ row }">
            <el-button
              v-if="!row.revoked_at && auth.can(PERM.apiKeyRevoke)"
              text
              type="danger"
              size="small"
              @click="onRevoke(row as ApiKeyRow)"
            >吊销</el-button>
          </template>
        </el-table-column>
        <template #empty>
          <el-empty description="还没有 API Key" />
        </template>
      </el-table>
    </el-card>

    <!-- 新建 -->
    <el-dialog v-model="dialog" title="新建 API Key" width="440px" :close-on-click-modal="false">
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
        <el-form-item label="名称" prop="name">
          <el-input v-model="form.name" placeholder="用途，如：CI 集成" />
        </el-form-item>
        <el-alert type="info" :closable="false">
          <p>Key 用于脚本 / 服务端调用租户 API 与 MCP 端点，权限等同创建者本人。</p>
        </el-alert>
      </el-form>
      <template #footer>
        <el-button @click="dialog = false">取消</el-button>
        <el-button type="primary" :loading="submitLoading" @click="onCreate">创建</el-button>
      </template>
    </el-dialog>

    <!-- secret 一次性展示 -->
    <el-dialog v-model="createdDialog" title="API Key 已创建" width="480px" :close-on-click-modal="false">
      <div v-if="created">
        <el-result icon="success" title="创建成功" :sub-title="created.name" />
        <el-alert type="warning" :closable="false" show-icon>
          <p>Secret <b>仅显示这一次</b>（服务端只存哈希），离开此页后将无法找回：</p>
        </el-alert>
        <div class="secret-box num">{{ created.secret }}</div>
      </div>
      <template #footer>
        <el-button @click="copySecret">复制 Secret</el-button>
        <el-button type="primary" @click="created = null">我已保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.page-title {
  margin: 0 0 6px;
  font-size: 18px;
  font-weight: 500;
}
.page-desc {
  margin: 0 0 16px;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}
.k-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}
.k-icon {
  color: var(--el-text-color-secondary);
}
.k-name {
  font-size: 13px;
  font-weight: 500;
}
.k-prefix {
  font-size: 12px;
  background: var(--el-fill-color-light);
  border-radius: 6px;
  padding: 3px 8px;
  color: var(--el-text-color-regular);
}
.k-sub {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.secret-box {
  margin-top: 12px;
  padding: 12px 16px;
  border-radius: 8px;
  background: var(--el-fill-color-light);
  border: 1px dashed var(--el-border-color);
  font-size: 14px;
  word-break: break-all;
  text-align: center;
}
</style>
