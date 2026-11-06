<script setup lang="ts">
import { Plus, Search } from "@element-plus/icons-vue";
import { createUser, listUsers, resetUserPassword, setUserRoles, updateUser } from "@/shared/api/tenant";
import { PERM } from "@/shared/permission/perms";
import { useAuthStore } from "@/shared/auth/store";
import { fmtDateTime } from "@/shared/utils/format";
import type { Paged, UserRow } from "@/shared/api/types";
import type { FormInstance, FormRules } from "element-plus";

const auth = useAuthStore();

const rows = ref<UserRow[]>([]);
const total = ref(0);
const loading = ref(false);
const query = reactive({ page: 1, size: 10, keyword: "", status: "" });

async function load() {
  loading.value = true;
  try {
    const p: Paged<UserRow> = await listUsers({
      page: query.page,
      size: query.size,
      keyword: query.keyword || undefined,
      status: query.status || undefined,
    });
    rows.value = p.items;
    total.value = p.total;
  } finally {
    loading.value = false;
  }
}
onMounted(load);

function onSearch() {
  query.page = 1;
  load();
}

/* ---------- 状态切换（含最后管理员保护演示） ---------- */

async function onToggleStatus(row: UserRow) {
  const next = row.status === "active" ? "disabled" : "active";
  const action = next === "active" ? "启用" : "禁用";
  try {
    if (next === "disabled") {
      await ElMessageBox.confirm(`禁用后「${row.display_name}」将立即无法登录，确定？`, "禁用成员", {
        type: "warning",
        confirmButtonText: "禁用",
      });
    }
    await updateUser(row.id, { status: next });
    row.status = next;
    ElMessage.success(`已${action}`);
  } catch {
    load();
  }
}

/* ---------- 角色调整 ---------- */

const roleDialog = ref(false);
const roleTarget = ref<UserRow | null>(null);
const roleCodes = ref<string[]>(["member"]);
const roleSaving = ref(false);

function openRoles(row: UserRow) {
  roleTarget.value = row;
  roleCodes.value = [...row.roles];
  roleDialog.value = true;
}

async function onSaveRoles() {
  if (!roleTarget.value || roleCodes.value.length === 0) {
    ElMessage.warning("至少保留一个角色");

    return;
  }
  roleSaving.value = true;
  try {
    await setUserRoles(roleTarget.value.id, roleCodes.value);
    ElMessage.success("角色已更新");
    roleDialog.value = false;
    load();
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : "保存失败");
  } finally {
    roleSaving.value = false;
  }
}

/* ---------- 重置密码 ---------- */

const resetTarget = ref<UserRow | null>(null);
const resetPassword = ref("");
const resetDialog = computed({
  get: () => resetPassword.value !== "",
  set: (v: boolean) => { if (!v) resetPassword.value = ""; },
});

async function onResetPassword(row: UserRow) {
  await ElMessageBox.confirm(`重置后「${row.display_name}」的当前密码立即失效，并需重新设置，确定？`, "重置密码", {
    type: "warning",
    confirmButtonText: "重置",
  }).catch(() => Promise.reject());
  try {
    const r = await resetUserPassword(row.id);
    resetTarget.value = row;
    resetPassword.value = r.initial_password;
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : "重置失败");
  }
}

async function copyResetPassword() {
  await navigator.clipboard.writeText(resetPassword.value).catch(() => undefined);
  ElMessage.success("已复制到剪贴板");
}

/* ---------- 邀请成员 ---------- */

const dialog = ref(false);
const submitLoading = ref(false);
const formRef = ref<FormInstance>();
const form = reactive({ email: "", display_name: "", role: "member" as "admin" | "member" });
const rules: FormRules = {
  email: [
    { required: true, message: "请输入邮箱", trigger: "blur" },
    { type: "email", message: "邮箱格式不正确", trigger: "blur" },
  ],
  display_name: [{ required: true, message: "请输入姓名", trigger: "blur" }],
};

const invited = ref<{ email: string; password: string } | null>(null);
const invitedDialog = computed({
  get: () => invited.value !== null,
  set: (v: boolean) => { if (!v) invited.value = null; },
});

async function onInvite() {
  const ok = await formRef.value?.validate().catch(() => false);
  if (!ok) return;
  submitLoading.value = true;
  try {
    const r = await createUser({ email: form.email, display_name: form.display_name, role: form.role });
    dialog.value = false;
    invited.value = { email: r.user.email, password: r.initial_password };
    Object.assign(form, { email: "", display_name: "", role: "member" });
    load();
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : "创建失败");
  } finally {
    submitLoading.value = false;
  }
}

async function copyPassword() {
  if (!invited.value) return;
  await navigator.clipboard.writeText(invited.value.password).catch(() => undefined);
  ElMessage.success("已复制到剪贴板");
}
</script>

<template>
  <div class="page">
    <h1 class="page-title">成员管理</h1>

    <div class="toolbar">
      <el-input
        v-model="query.keyword"
        placeholder="搜索姓名 / 邮箱"
        :prefix-icon="Search"
        clearable
        style="width: 240px"
        @clear="onSearch"
        @keyup.enter="onSearch"
      />
      <el-select v-model="query.status" placeholder="全部状态" clearable style="width: 140px" @change="onSearch">
        <el-option label="正常" value="active" />
        <el-option label="已禁用" value="disabled" />
      </el-select>
      <el-button @click="onSearch">查询</el-button>
      <div class="toolbar-spacer" />
      <el-button v-if="auth.can(PERM.userCreate)" v-permission="PERM.userCreate" type="primary" :icon="Plus" @click="dialog = true">
        邀请成员
      </el-button>
    </div>

    <el-card shadow="never">
      <el-table v-loading="loading" :data="rows" style="width: 100%">
        <el-table-column label="成员" min-width="220">
          <template #default="{ row }">
            <div class="u-cell">
              <el-avatar :size="32" class="u-avatar">{{ row.display_name.slice(0, 1) }}</el-avatar>
              <div>
                <p class="u-name">{{ row.display_name }}</p>
                <p class="u-sub">{{ row.email }}</p>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="角色" width="110">
          <template #default="{ row }">
            <el-tag :type="row.roles.includes('admin') ? 'danger' : 'info'" size="small" class="status-pill">
              {{ row.roles.includes("admin") ? "管理员" : "成员" }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="110">
          <template #default="{ row }">
            <el-switch
              :model-value="row.status === 'active'"
              inline-prompt
              active-text="启"
              inactive-text="停"
              :disabled="!auth.can(PERM.userUpdate)"
              @change="onToggleStatus(row as UserRow)"
            />
          </template>
        </el-table-column>
        <el-table-column label="创建时间" width="170">
          <template #default="{ row }"><span class="num u-sub">{{ fmtDateTime(row.created_at) }}</span></template>
        </el-table-column>
        <el-table-column label="操作" width="160" fixed="right">
          <template #default="{ row }">
            <el-button
              v-if="auth.can(PERM.userUpdate)"
              text
              type="primary"
              size="small"
              @click="openRoles(row as UserRow)"
            >角色</el-button>
            <el-button
              v-if="auth.can(PERM.userUpdate)"
              text
              size="small"
              @click="onResetPassword(row as UserRow)"
            >重置密码</el-button>
          </template>
        </el-table-column>
        <template #empty>
          <el-empty description="暂无成员" />
        </template>
      </el-table>

      <div class="page-pagination">
        <el-pagination
          v-model:current-page="query.page"
          v-model:page-size="query.size"
          :total="total"
          layout="total, prev, pager, next"
          @current-change="load"
        />
      </div>
    </el-card>

    <!-- 邀请成员 -->
    <el-dialog v-model="dialog" title="邀请成员" width="460px" :close-on-click-modal="false">
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
        <el-form-item label="邮箱" prop="email">
          <el-input v-model="form.email" placeholder="name@acme.com" />
        </el-form-item>
        <el-form-item label="姓名" prop="display_name">
          <el-input v-model="form.display_name" placeholder="真实姓名" />
        </el-form-item>
        <el-form-item label="角色">
          <el-radio-group v-model="form.role">
            <el-radio value="member">成员（默认）</el-radio>
            <el-radio value="admin">管理员</el-radio>
          </el-radio-group>
          <div class="role-hint">内置角色：管理员拥有全部权限，成员为只读基线权限</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialog = false">取消</el-button>
        <el-button type="primary" :loading="submitLoading" @click="onInvite">邀请</el-button>
      </template>
    </el-dialog>

    <!-- 初始密码一次性展示 -->
    <el-dialog v-model="invitedDialog" title="成员已创建" width="440px" :close-on-click-modal="false">
      <div v-if="invited">
        <el-result icon="success" title="创建成功" :sub-title="invited.email" />
        <el-alert type="warning" :closable="false" show-icon>
          <p>初始密码（<b>仅显示这一次</b>，请发给成员本人）：</p>
        </el-alert>
        <div class="password-box num">{{ invited.password }}</div>
      </div>
      <template #footer>
        <el-button @click="copyPassword">复制密码</el-button>
        <el-button type="primary" @click="invited = null">完成</el-button>
      </template>
    </el-dialog>

    <!-- 角色调整 -->
    <el-dialog v-model="roleDialog" title="调整角色" width="420px" :close-on-click-modal="false">
      <p class="role-target">{{ roleTarget?.display_name }} · {{ roleTarget?.email }}</p>
      <el-checkbox-group v-model="roleCodes">
        <el-checkbox value="member">成员（只读基线权限）</el-checkbox>
        <el-checkbox value="admin">管理员（全部权限）</el-checkbox>
      </el-checkbox-group>
      <el-alert type="info" :closable="false" style="margin-top: 12px">
        <p>去掉管理员会立即失去管理能力；系统不允许移除最后一个启用中的管理员。</p>
      </el-alert>
      <template #footer>
        <el-button @click="roleDialog = false">取消</el-button>
        <el-button type="primary" :loading="roleSaving" @click="onSaveRoles">保存</el-button>
      </template>
    </el-dialog>

    <!-- 重置密码一次性展示 -->
    <el-dialog v-model="resetDialog" title="密码已重置" width="440px" :close-on-click-modal="false">
      <el-result icon="success" title="重置成功" :sub-title="resetTarget?.email" />
      <el-alert type="warning" :closable="false" show-icon>
        <p>新初始密码（<b>仅显示这一次</b>），成员下次登录需再次修改：</p>
      </el-alert>
      <div class="password-box num">{{ resetPassword }}</div>
      <template #footer>
        <el-button @click="copyResetPassword">复制密码</el-button>
        <el-button type="primary" @click="resetPassword = ''">完成</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.page-title {
  margin: 0 0 16px;
  font-size: 18px;
  font-weight: 500;
}
.u-cell {
  display: flex;
  align-items: center;
  gap: 10px;
}
.u-avatar {
  background: var(--el-color-primary-light-8);
  color: var(--el-color-primary);
  font-size: 13px;
  flex-shrink: 0;
}
.u-name {
  margin: 0;
  font-size: 13px;
  font-weight: 500;
  color: var(--el-text-color-primary);
}
.u-sub {
  margin: 2px 0 0;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.role-hint {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  line-height: 1.5;
  margin-top: 6px;
}
.password-box {
  margin-top: 12px;
  padding: 12px 16px;
  border-radius: 8px;
  background: var(--el-fill-color-light);
  border: 1px dashed var(--el-border-color);
  font-size: 16px;
  letter-spacing: 1px;
  text-align: center;
}
</style>
