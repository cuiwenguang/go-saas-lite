<script setup lang="ts">
import { Plus, Search } from "@element-plus/icons-vue";
import { inviteUser, listUsers, removeUser, setUserStatus } from "@/shared/api/tenant";
import { PERM } from "@/shared/permission/perms";
import { useAuthStore } from "@/shared/auth/store";
import { fmtDateTime } from "@/shared/utils/format";
import type { Paged, UserRow } from "@/shared/api/types";
import type { FormInstance, FormRules } from "element-plus";

const auth = useAuthStore();

const rows = ref<UserRow[]>([]);
const total = ref(0);
const loading = ref(false);
const query = reactive({ page: 1, page_size: 10, keyword: "", status: "" });

async function load() {
  loading.value = true;
  try {
    const p: Paged<UserRow> = await listUsers({
      ...query,
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
    row.status = next;
    await setUserStatus(row.id, next);
    ElMessage.success(`已${action}`);
  } catch (e) {
    row.status = row.status; // 还原
    if (e instanceof Error && e.message) load();
  }
}

async function onRemove(row: UserRow) {
  await ElMessageBox.confirm(`删除后「${row.display_name}」的所有角色绑定一并移除，确定？`, "删除成员", {
    type: "warning",
    confirmButtonText: "删除",
    confirmButtonClass: "el-button--danger",
  }).catch(() => Promise.reject());
  try {
    await removeUser(row.id);
    ElMessage.success("已删除");
    load();
  } catch (e) {
    // mock 会返回 40009：不能删除最后一个管理员
    ElMessage.error(e instanceof Error ? e.message : "删除失败");
  }
}

/* ---------- 邀请成员 ---------- */

const dialog = ref(false);
const submitLoading = ref(false);
const formRef = ref<FormInstance>();
const form = reactive({ email: "", display_name: "", roles: "member" });
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
    const r = await inviteUser({ email: form.email, display_name: form.display_name, roles: [form.roles] });
    dialog.value = false;
    invited.value = { email: r.user.email, password: r.initial_password };
    Object.assign(form, { email: "", display_name: "", roles: "member" });
    load();
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : "邀请失败");
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
        <el-table-column label="创建时间" width="160">
          <template #default="{ row }"><span class="num u-sub">{{ fmtDateTime(row.created_at) }}</span></template>
        </el-table-column>
        <el-table-column label="最近登录" width="160">
          <template #default="{ row }"><span class="num u-sub">{{ fmtDateTime(row.last_login_at) }}</span></template>
        </el-table-column>
        <el-table-column label="操作" width="90" fixed="right">
          <template #default="{ row }">
            <el-button
              v-if="auth.can(PERM.userDelete)"
              text
              type="danger"
              size="small"
              @click="onRemove(row as UserRow)"
            >删除</el-button>
          </template>
        </el-table-column>
        <template #empty>
          <el-empty description="暂无成员" />
        </template>
      </el-table>

      <div class="page-pagination">
        <el-pagination
          v-model:current-page="query.page"
          v-model:page-size="query.page_size"
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
          <el-radio-group v-model="form.roles">
            <el-radio value="member">成员（默认）</el-radio>
            <el-radio value="admin">管理员</el-radio>
          </el-radio-group>
          <div class="role-hint">v1 仅内置角色：管理员拥有全部权限，成员为只读基线权限</div>
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
        <el-result icon="success" title="邀请成功" :sub-title="invited.email" />
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
