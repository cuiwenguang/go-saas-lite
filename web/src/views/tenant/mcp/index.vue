<script setup lang="ts">
import { listMcpServers, listMcpTools, setToolEnabled } from "@/shared/api/tenant";
import { PERM } from "@/shared/permission/perms";
import { useAuthStore } from "@/shared/auth/store";
import { fmtNumber } from "@/shared/utils/format";
import type { McpServerRow, McpToolRow, Paged } from "@/shared/api/types";

const auth = useAuthStore();

const servers = ref<McpServerRow[]>([]);
const tools = ref<McpToolRow[]>([]);
const loading = ref(false);
const selected = ref<string>("");

onMounted(async () => {
  servers.value = await listMcpServers();
  await loadTools();
});

async function loadTools() {
  loading.value = true;
  try {
    const p: Paged<McpToolRow> = await listMcpTools({ page: 1, page_size: 50, server_id: selected.value || undefined });
    tools.value = p.items;
  } finally {
    loading.value = false;
  }
}

function selectServer(id: string) {
  selected.value = selected.value === id ? "" : id;
  loadTools();
}

async function onToggle(tool: McpToolRow) {
  try {
    const next = !tool.enabled;
    await setToolEnabled(tool.id, next);
    tool.enabled = next;
    ElMessage.success(next ? `已启用 ${tool.name}` : `已停用 ${tool.name}`);
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : "操作失败");
  }
}

const serverDot: Record<string, string> = { connected: "var(--el-color-success)", error: "var(--el-color-danger)" };
const serverText: Record<string, string> = { connected: "已连接", error: "连接异常" };
</script>

<template>
  <div class="page">
    <h1 class="page-title">MCP 服务</h1>
    <p class="page-desc">
      上游 MCP 服务注册表。每个工具绑定平台权限点 —— 用户在 Web 端没有的权限，通过 Agent 调用同样拿不到。
    </p>

    <el-row :gutter="16">
      <el-col :xs="24" :lg="7">
        <el-card shadow="never" class="server-card">
          <template #header>上游服务</template>
          <div
            v-for="s in servers"
            :key="s.id"
            class="server"
            :class="{ active: selected === s.id }"
            @click="selectServer(s.id)"
          >
            <div class="server-head">
              <span class="dot" :style="{ background: serverDot[s.status] }" />
              <span class="server-name">{{ s.name }}</span>
              <span class="server-count num">{{ s.tool_count }} 工具</span>
            </div>
            <p class="server-url">{{ s.url }}</p>
            <p class="server-desc">{{ s.description }}</p>
            <el-tag :type="s.status === 'connected' ? 'success' : 'danger'" size="small" class="status-pill">
              {{ serverText[s.status] }}
            </el-tag>
          </div>
        </el-card>
      </el-col>

      <el-col :xs="24" :lg="17">
        <el-card shadow="never">
          <template #header>
            <div class="tools-head">
              <span>工具注册表</span>
              <el-tag v-if="selected" size="small" closable @close="selectServer(selected)">
                {{ servers.find((s) => s.id === selected)?.name }}
              </el-tag>
            </div>
          </template>
          <el-table v-loading="loading" :data="tools" style="width: 100%">
            <el-table-column label="工具" min-width="200">
              <template #default="{ row }">
                <p class="tool-name num">{{ row.name }}</p>
                <p class="tool-desc">{{ row.description }}</p>
              </template>
            </el-table-column>
            <el-table-column label="所属服务" width="120">
              <template #default="{ row }"><span class="tool-sub">{{ row.server_name }}</span></template>
            </el-table-column>
            <el-table-column label="所需权限" min-width="170">
              <template #default="{ row }">
                <el-tag v-for="p in row.required_perms" :key="p" size="small" effect="plain" class="perm-tag">{{ p }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="30 天调用" width="110" align="right">
              <template #default="{ row }"><span class="num tool-sub">{{ fmtNumber(row.calls_30d) }}</span></template>
            </el-table-column>
            <el-table-column label="启用" width="90">
              <template #default="{ row }">
                <el-switch
                  :model-value="row.enabled"
                  inline-prompt
                  :disabled="!auth.can(PERM.mcpUpdate)"
                  @change="onToggle(row as McpToolRow)"
                />
              </template>
            </el-table-column>
            <template #empty>
              <el-empty description="暂无工具" />
            </template>
          </el-table>
        </el-card>
      </el-col>
    </el-row>
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
.server {
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 10px;
  padding: 12px 14px;
  margin-bottom: 12px;
  cursor: pointer;
  transition: border-color 0.15s;
}
.server:hover {
  border-color: var(--el-color-primary-light-5);
}
.server.active {
  border-color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}
.server-head {
  display: flex;
  align-items: center;
  gap: 8px;
}
.dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}
.server-name {
  font-size: 13px;
  font-weight: 500;
}
.server-count {
  margin-left: auto;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.server-url {
  margin: 6px 0 2px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  word-break: break-all;
}
.server-desc {
  margin: 0 0 8px;
  font-size: 12px;
  color: var(--el-text-color-regular);
}
.tool-name {
  margin: 0;
  font-size: 13px;
  font-weight: 500;
  font-family: var(--el-font-family);
}
.tool-desc {
  margin: 2px 0 0;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.tool-sub {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.perm-tag {
  margin-right: 6px;
  font-family: ui-monospace, monospace;
}
.tools-head {
  display: flex;
  align-items: center;
  gap: 8px;
}
:deep(.el-card__header) {
  padding: 14px 20px;
  font-size: 14px;
  font-weight: 500;
  border-bottom: 1px solid var(--el-border-color-lighter);
}
</style>
