<script setup lang="ts">
import { Plus } from "@element-plus/icons-vue";
import {
  createMcpServer, deleteMcpServer, listMcpCalls, listMcpServers, listMcpTools, updateMcpServer, updateMcpTool,
} from "@/shared/api/tenant";
import { PERM } from "@/shared/permission/perms";
import { useAuthStore } from "@/shared/auth/store";
import { fmtDateTime, fmtNumber } from "@/shared/utils/format";
import type { McpCallRow, McpServerRow, McpToolRow, Paged } from "@/shared/api/types";
import type { FormInstance, FormRules } from "element-plus";

const auth = useAuthStore();

const tab = ref<"servers" | "tools" | "calls">("servers");

/* ---------- 上游服务 ---------- */

const servers = ref<McpServerRow[]>([]);
const serverLoading = ref(false);

async function loadServers() {
  serverLoading.value = true;
  try {
    const r = await listMcpServers();
    servers.value = r.items;
  } finally {
    serverLoading.value = false;
  }
}

const dialog = ref(false);
const saving = ref(false);
const formRef = ref<FormInstance>();
const serverForm = ref({ id: "", name: "", base_url: "", credential: "", enabled: true });
const rules: FormRules = {
  name: [{ required: true, message: "请输入名称", trigger: "blur" }],
  base_url: [
    { required: true, message: "请输入上游地址", trigger: "blur" },
    { pattern: /^https?:\/\//, message: "必须是 http(s) 地址", trigger: "blur" },
  ],
};

function openCreate() {
  serverForm.value = { id: "", name: "", base_url: "", credential: "", enabled: true };
  dialog.value = true;
}

function openEdit(row: McpServerRow) {
  serverForm.value = { id: row.id, name: row.name, base_url: row.base_url, credential: "", enabled: row.enabled };
  dialog.value = true;
}

async function onSave() {
  const ok = await formRef.value?.validate().catch(() => false);
  if (!ok) return;
  saving.value = true;
  const current = serverForm.value;
  try {
    if (current.id) {
      await updateMcpServer(current.id, {
        name: current.name,
        base_url: current.base_url,
        credential: current.credential === "" ? undefined : current.credential,
        enabled: current.enabled,
      });
    } else {
      await createMcpServer({
        name: current.name,
        base_url: current.base_url,
        credential: current.credential || undefined,
        enabled: current.enabled,
      });
    }
    dialog.value = false;
    ElMessage.success("已保存");
    await loadServers();
    await loadTools();
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : "保存失败");
  } finally {
    saving.value = false;
  }
}

async function onToggleServer(row: McpServerRow) {
  try {
    await updateMcpServer(row.id, { enabled: !row.enabled });
    row.enabled = !row.enabled;
    ElMessage.success(row.enabled ? "已启用" : "已停用");
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : "操作失败");
  }
}

async function onDelete(row: McpServerRow) {
  await ElMessageBox.confirm(
    `删除「${row.name}」会同时移除它下面的 ${row.tool_count} 个工具配置，确定？`,
    "删除上游服务",
    { type: "warning", confirmButtonText: "删除", confirmButtonClass: "el-button--danger" },
  ).catch(() => Promise.reject());
  try {
    await deleteMcpServer(row.id);
    ElMessage.success("已删除");
    await loadServers();
    await loadTools();
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : "删除失败");
  }
}

/* ---------- 工具 ---------- */

const tools = ref<McpToolRow[]>([]);
const toolLoading = ref(false);
const serverFilter = ref("");

async function loadTools() {
  toolLoading.value = true;
  try {
    const r = await listMcpTools({ server_id: serverFilter.value || undefined });
    tools.value = r.items;
  } finally {
    toolLoading.value = false;
  }
}

function serverName(id: string) {
  return servers.value.find((s) => s.id === id)?.name || id;
}

async function onToggleTool(row: McpToolRow) {
  try {
    await updateMcpTool(row.id, { enabled: !row.enabled });
    row.enabled = !row.enabled;
    ElMessage.success(row.enabled ? `已启用 ${row.name}` : `已停用 ${row.name}`);
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : "操作失败");
  }
}

/* 所需权限：逗号分隔输入，后端会校验权限点是否已登记 */
const permDialog = ref(false);
const permTarget = ref<McpToolRow | null>(null);
const permText = ref("");
const permSaving = ref(false);

function openPerms(row: McpToolRow) {
  permTarget.value = row;
  permText.value = row.required_perms.join(", ");
  permDialog.value = true;
}

async function onSavePerms() {
  if (!permTarget.value) return;
  permSaving.value = true;
  try {
    const codes = permText.value.split(",").map((s) => s.trim()).filter(Boolean);
    const updated = await updateMcpTool(permTarget.value.id, { required_perms: codes });
    permTarget.value.required_perms = updated.required_perms;
    permDialog.value = false;
    ElMessage.success("已更新所需权限");
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : "保存失败");
  } finally {
    permSaving.value = false;
  }
}

/* ---------- 调用日志 ---------- */

const calls = ref<McpCallRow[]>([]);
const callTotal = ref(0);
const callLoading = ref(false);
const callQuery = reactive({ page: 1, size: 20, status: "", tool: "" });

async function loadCalls() {
  callLoading.value = true;
  try {
    const p: Paged<McpCallRow> = await listMcpCalls({
      page: callQuery.page,
      size: callQuery.size,
      status: callQuery.status || undefined,
      tool: callQuery.tool || undefined,
    });
    calls.value = p.items;
    callTotal.value = p.total;
  } finally {
    callLoading.value = false;
  }
}

function onTabChange(name: string | number) {
  if (name === "tools" && tools.value.length === 0) loadTools();
  if (name === "calls") loadCalls();
}

onMounted(async () => {
  await loadServers();
  await loadTools();
});
</script>

<template>
  <div class="page">
    <h1 class="page-title">MCP 服务</h1>
    <p class="page-desc">
      上游 MCP 服务与工具注册表。每个工具可绑定权限点 —— 用户在 Web 端没有的权限，通过 Agent 调用同样拿不到。
    </p>

    <el-tabs v-model="tab" class="mcp-tabs" @tab-change="onTabChange">
      <el-tab-pane label="上游服务" name="servers">
        <div class="toolbar">
          <div class="toolbar-spacer" />
          <el-button v-if="auth.can(PERM.mcpUpdate)" type="primary" :icon="Plus" @click="openCreate">新建服务</el-button>
        </div>

        <el-card shadow="never">
          <el-table v-loading="serverLoading" :data="servers" style="width: 100%">
            <el-table-column label="名称" min-width="180">
              <template #default="{ row }">
                <p class="tool-name">{{ row.name }}</p>
                <p class="tool-desc">{{ row.base_url }}</p>
              </template>
            </el-table-column>
            <el-table-column label="凭据" width="110">
              <template #default="{ row }">
                <el-tag :type="row.has_credential ? 'success' : 'info'" size="small" effect="plain">
                  {{ row.has_credential ? "已配置" : "无" }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="工具" width="90" align="right">
              <template #default="{ row }"><span class="num tool-sub">{{ row.tool_count }}</span></template>
            </el-table-column>
            <el-table-column label="状态" width="110">
              <template #default="{ row }">
                <el-switch
                  :model-value="row.enabled"
                  inline-prompt
                  active-text="启"
                  inactive-text="停"
                  :disabled="!auth.can(PERM.mcpUpdate)"
                  @change="onToggleServer(row as McpServerRow)"
                />
              </template>
            </el-table-column>
            <el-table-column label="更新时间" width="170">
              <template #default="{ row }"><span class="num tool-sub">{{ fmtDateTime(row.updated_at) }}</span></template>
            </el-table-column>
            <el-table-column label="操作" width="130" fixed="right">
              <template #default="{ row }">
                <el-button v-if="auth.can(PERM.mcpUpdate)" text size="small" @click="openEdit(row as McpServerRow)">编辑</el-button>
                <el-button v-if="auth.can(PERM.mcpUpdate)" text type="danger" size="small" @click="onDelete(row as McpServerRow)">删除</el-button>
              </template>
            </el-table-column>
            <template #empty>
              <el-empty description="还没有上游 MCP 服务" />
            </template>
          </el-table>
        </el-card>
      </el-tab-pane>

      <el-tab-pane label="工具注册表" name="tools">
        <div class="toolbar">
          <el-select v-model="serverFilter" placeholder="全部服务" clearable style="width: 200px" @change="loadTools">
            <el-option v-for="s in servers" :key="s.id" :label="s.name" :value="s.id" />
          </el-select>
        </div>

        <el-card shadow="never">
          <el-table v-loading="toolLoading" :data="tools" style="width: 100%">
            <el-table-column label="工具" min-width="200">
              <template #default="{ row }">
                <p class="tool-name num">{{ row.name }}</p>
                <p class="tool-desc">{{ row.description || "—" }}</p>
              </template>
            </el-table-column>
            <el-table-column label="所属服务" width="140">
              <template #default="{ row }"><span class="tool-sub">{{ serverName(row.server_id) }}</span></template>
            </el-table-column>
            <el-table-column label="所需权限" min-width="200">
              <template #default="{ row }">
                <el-tag v-for="p in row.required_perms" :key="p" size="small" effect="plain" class="perm-tag">{{ p }}</el-tag>
                <span v-if="row.required_perms.length === 0" class="tool-sub">不限</span>
              </template>
            </el-table-column>
            <el-table-column label="启用" width="90">
              <template #default="{ row }">
                <el-switch
                  :model-value="row.enabled"
                  inline-prompt
                  :disabled="!auth.can(PERM.mcpUpdate)"
                  @change="onToggleTool(row as McpToolRow)"
                />
              </template>
            </el-table-column>
            <el-table-column label="操作" width="110" fixed="right">
              <template #default="{ row }">
                <el-button v-if="auth.can(PERM.mcpUpdate)" text size="small" @click="openPerms(row as McpToolRow)">权限</el-button>
              </template>
            </el-table-column>
            <template #empty>
              <el-empty description="暂无工具配置" />
            </template>
          </el-table>
        </el-card>
      </el-tab-pane>

      <el-tab-pane label="调用日志" name="calls">
        <div class="toolbar">
          <el-input v-model="callQuery.tool" placeholder="工具名" clearable style="width: 200px" @clear="loadCalls" />
          <el-select v-model="callQuery.status" placeholder="全部结果" clearable style="width: 150px" @change="loadCalls">
            <el-option label="成功" value="ok" />
            <el-option label="失败" value="error" />
          </el-select>
          <el-button @click="loadCalls">查询</el-button>
        </div>

        <el-card shadow="never">
          <el-table v-loading="callLoading" :data="calls" style="width: 100%">
            <el-table-column label="时间" width="170">
              <template #default="{ row }"><span class="num tool-sub">{{ fmtDateTime(row.created_at) }}</span></template>
            </el-table-column>
            <el-table-column label="工具" min-width="160" prop="tool_name" />
            <el-table-column label="客户端" width="160" prop="client_id" />
            <el-table-column label="结果" width="100">
              <template #default="{ row }">
                <el-tag :type="row.status === 'ok' ? 'success' : 'danger'" size="small" class="status-pill">
                  {{ row.status === "ok" ? "成功" : "失败" }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="耗时" width="110" align="right">
              <template #default="{ row }"><span class="num tool-sub">{{ fmtNumber(row.duration_ms) }} ms</span></template>
            </el-table-column>
            <el-table-column label="错误码" width="130">
              <template #default="{ row }"><span class="num tool-sub">{{ row.error_code || "—" }}</span></template>
            </el-table-column>
            <template #empty>
              <el-empty description="暂无调用记录" />
            </template>
          </el-table>

          <div class="page-pagination">
            <el-pagination
              v-model:current-page="callQuery.page"
              v-model:page-size="callQuery.size"
              :total="callTotal"
              layout="total, prev, pager, next"
              @current-change="loadCalls"
            />
          </div>
        </el-card>
      </el-tab-pane>
    </el-tabs>

    <!-- 服务编辑 -->
    <el-dialog v-model="dialog" :title="serverForm.id ? '编辑上游服务' : '新建上游服务'" width="480px" :close-on-click-modal="false">
      <el-form ref="formRef" :model="serverForm" :rules="rules" label-position="top">
        <el-form-item label="名称" prop="name">
          <el-input v-model="serverForm.name" placeholder="如：内部工具网关" />
        </el-form-item>
        <el-form-item label="上游地址" prop="base_url">
          <el-input v-model="serverForm.base_url" placeholder="https://mcp.example.com/rpc" />
        </el-form-item>
        <el-form-item label="凭据（Bearer Token）">
          <el-input
            v-model="serverForm.credential"
            type="password"
            show-password
            :placeholder="serverForm.id ? '留空表示不修改' : '可留空'"
          />
          <div class="role-hint">服务端加密存储，接口只回显「是否已配置」。</div>
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="serverForm.enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialog = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="onSave">保存</el-button>
      </template>
    </el-dialog>

    <!-- 工具权限 -->
    <el-dialog v-model="permDialog" title="所需权限" width="460px" :close-on-click-modal="false">
      <p class="tool-name num">{{ permTarget?.name }}</p>
      <el-input v-model="permText" type="textarea" :rows="3" placeholder="如：mcp:read, user:read（留空表示不限）" />
      <el-alert type="info" :closable="false" style="margin-top: 12px">
        <p>权限点必须已登记，否则保存会被拒绝；调用方缺少任一权限点即被拒绝。</p>
      </el-alert>
      <template #footer>
        <el-button @click="permDialog = false">取消</el-button>
        <el-button type="primary" :loading="permSaving" @click="onSavePerms">保存</el-button>
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
