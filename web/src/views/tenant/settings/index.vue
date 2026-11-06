<script setup lang="ts">
import { Plus } from "@element-plus/icons-vue";
import { listConfigs, removeConfig, setConfig } from "@/shared/api/tenant";
import { PERM } from "@/shared/permission/perms";
import { useAuthStore } from "@/shared/auth/store";
import { fmtDateTime } from "@/shared/utils/format";
import type { ConfigRow } from "@/shared/api/types";
import type { FormInstance, FormRules } from "element-plus";

const auth = useAuthStore();

const namespaces = ["mcp", "feature", "brand", "integration"];
const namespace = ref("mcp");
const rows = ref<ConfigRow[]>([]);
const loading = ref(false);

async function load() {
  loading.value = true;
  try {
    const r = await listConfigs(namespace.value);
    rows.value = r.items;
  } finally {
    loading.value = false;
  }
}
onMounted(load);

/* ---------- 新增 / 编辑 ---------- */

const dialog = ref(false);
const saving = ref(false);
const editing = ref(false);
const formRef = ref<FormInstance>();
const configForm = ref({ key: "", value: "" });
const rules: FormRules = {
  key: [{ required: true, message: "请输入配置名", trigger: "blur" }],
  value: [{ required: true, message: "请输入配置值", trigger: "blur" }],
};

function openCreate() {
  editing.value = false;
  configForm.value = { key: "", value: "" };
  dialog.value = true;
}

function openEdit(row: ConfigRow) {
  editing.value = true;
  configForm.value = { key: row.key, value: row.value };
  dialog.value = true;
}

async function onSave() {
  const ok = await formRef.value?.validate().catch(() => false);
  if (!ok) return;
  saving.value = true;
  try {
    const current = configForm.value;
    await setConfig(namespace.value, current.key, current.value);
    dialog.value = false;
    ElMessage.success("已保存");
    load();
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : "保存失败");
  } finally {
    saving.value = false;
  }
}

async function onRemove(row: ConfigRow) {
  await ElMessageBox.confirm(`删除配置「${row.key}」后，依赖它的功能会回退到默认值。确定？`, "删除配置", {
    type: "warning",
    confirmButtonText: "删除",
    confirmButtonClass: "el-button--danger",
  }).catch(() => Promise.reject());
  try {
    await removeConfig(namespace.value, row.key);
    ElMessage.success("已删除");
    load();
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : "删除失败");
  }
}
</script>

<template>
  <div class="page">
    <h1 class="page-title">租户配置</h1>
    <p class="page-desc">按命名空间存放的键值配置，修改后即时生效（同一租户内全局可见）。</p>

    <div class="toolbar">
      <el-select v-model="namespace" style="width: 200px" @change="load">
        <el-option v-for="ns in namespaces" :key="ns" :label="ns" :value="ns" />
      </el-select>
      <div class="toolbar-spacer" />
      <el-button v-if="auth.can(PERM.tenantConfUpdate)" type="primary" :icon="Plus" @click="openCreate">
        新增配置
      </el-button>
    </div>

    <el-card shadow="never">
      <el-table v-loading="loading" :data="rows" style="width: 100%">
        <el-table-column label="配置名" min-width="180">
          <template #default="{ row }"><span class="num c-key">{{ row.key }}</span></template>
        </el-table-column>
        <el-table-column label="值" min-width="240">
          <template #default="{ row }"><span class="num c-val">{{ row.value }}</span></template>
        </el-table-column>
        <el-table-column label="更新时间" width="170">
          <template #default="{ row }"><span class="num c-sub">{{ fmtDateTime(row.updated_at) }}</span></template>
        </el-table-column>
        <el-table-column label="操作" width="130" fixed="right">
          <template #default="{ row }">
            <template v-if="auth.can(PERM.tenantConfUpdate)">
              <el-button text size="small" @click="openEdit(row as ConfigRow)">编辑</el-button>
              <el-button text type="danger" size="small" @click="onRemove(row as ConfigRow)">删除</el-button>
            </template>
          </template>
        </el-table-column>
        <template #empty>
          <el-empty description="该命名空间下还没有配置" />
        </template>
      </el-table>
    </el-card>

    <el-dialog v-model="dialog" :title="editing ? '编辑配置' : '新增配置'" width="480px" :close-on-click-modal="false">
      <el-form ref="formRef" :model="configForm" :rules="rules" label-position="top">
        <el-form-item label="配置名" prop="key">
          <el-input v-model="configForm.key" :disabled="editing" placeholder="如：upstream_timeout" />
        </el-form-item>
        <el-form-item label="值" prop="value">
          <el-input v-model="configForm.value" type="textarea" :rows="3" placeholder="字符串，JSON 请自行保证格式" />
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
  margin: 0 0 6px;
  font-size: 18px;
  font-weight: 500;
}
.page-desc {
  margin: 0 0 16px;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}
.c-key {
  font-size: 13px;
  font-weight: 500;
}
.c-val {
  font-size: 12px;
  color: var(--el-text-color-regular);
  word-break: break-all;
}
.c-sub {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
</style>
