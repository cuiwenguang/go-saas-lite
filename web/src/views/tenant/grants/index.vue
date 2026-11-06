<script setup lang="ts">
import { listGrants, revokeGrant } from "@/shared/api/tenant";
import { PERM } from "@/shared/permission/perms";
import { useAuthStore } from "@/shared/auth/store";
import { fmtDateTime } from "@/shared/utils/format";
import type { OAuthGrantRow } from "@/shared/api/types";

const auth = useAuthStore();

const rows = ref<OAuthGrantRow[]>([]);
const loading = ref(false);
const includeRevoked = ref(false);

async function load() {
  loading.value = true;
  try {
    const r = await listGrants(includeRevoked.value);
    rows.value = r.items;
  } finally {
    loading.value = false;
  }
}
onMounted(load);

async function onRevoke(row: OAuthGrantRow) {
  await ElMessageBox.confirm(
    `撤销后「${row.client_name}」无法再用刷新令牌换取访问令牌，已签发的访问令牌到期即失效。确定？`,
    "撤销授权",
    { type: "warning", confirmButtonText: "撤销", confirmButtonClass: "el-button--danger" },
  ).catch(() => Promise.reject());
  try {
    await revokeGrant(row.client_id);
    ElMessage.success("已撤销");
    load();
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : "撤销失败");
  }
}
</script>

<template>
  <div class="page">
    <h1 class="page-title">Agent 授权</h1>
    <p class="page-desc">
      通过 OAuth 接入的 Agent 客户端列表。撤销后不可再用刷新令牌续期，访问令牌最长 30 分钟后失效。
    </p>

    <div class="toolbar">
      <el-switch v-model="includeRevoked" active-text="显示已撤销" @change="load" />
      <div class="toolbar-spacer" />
    </div>

    <el-card shadow="never">
      <el-table v-loading="loading" :data="rows" style="width: 100%">
        <el-table-column label="客户端" min-width="200">
          <template #default="{ row }">
            <p class="g-name">{{ row.client_name }}</p>
            <p class="g-sub num">{{ row.client_id }}</p>
          </template>
        </el-table-column>
        <el-table-column label="Scope" width="120" prop="scope" />
        <el-table-column label="状态" width="110">
          <template #default="{ row }">
            <el-tag :type="row.revoked_at ? 'info' : 'success'" size="small" class="status-pill">
              {{ row.revoked_at ? "已撤销" : "生效中" }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="授权时间" width="170">
          <template #default="{ row }"><span class="num g-sub">{{ fmtDateTime(row.created_at) }}</span></template>
        </el-table-column>
        <el-table-column label="操作" width="100" fixed="right">
          <template #default="{ row }">
            <el-button
              v-if="!row.revoked_at && auth.can(PERM.oauthGrantRevoke)"
              text
              type="danger"
              size="small"
              @click="onRevoke(row as OAuthGrantRow)"
            >撤销</el-button>
          </template>
        </el-table-column>
        <template #empty>
          <el-empty description="该账号还没有 Agent 授权" />
        </template>
      </el-table>
    </el-card>
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
.g-name {
  margin: 0;
  font-size: 13px;
  font-weight: 500;
}
.g-sub {
  margin: 2px 0 0;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
</style>
