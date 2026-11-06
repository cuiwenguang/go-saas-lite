<script setup lang="ts">
import { MoreFilled, Plus, Refresh, Search } from "@element-plus/icons-vue";
import {
  archiveTenant, createTenant, listPlans, listTenants, setTenantStatus, tenantSummary,
  type TenantSummary,
} from "@/shared/api/platform";
import { PERM } from "@/shared/permission/perms";
import { useAuthStore } from "@/shared/auth/store";
import { fmtDateTime, fromNow, untilNow } from "@/shared/utils/format";
import type { Paged, Plan, TenantRow, TenantStatus } from "@/shared/api/types";
import type { FormInstance, FormRules } from "element-plus";

const auth = useAuthStore();
const router = useRouter();

/* ---------- 列表 ---------- */

const rows = ref<TenantRow[]>([]);
const total = ref(0);
const loading = ref(false);
const summary = ref<TenantSummary>({ total: 0, pending: 0, active: 0, suspended: 0, expired: 0, archived: 0 });
const query = reactive({ page: 1, size: 10, keyword: "", status: "" });

async function load() {
  loading.value = true;
  try {
    const [p, s] = await Promise.all([
      listTenants({ ...query, keyword: query.keyword || undefined, status: query.status || undefined }),
      tenantSummary(),
    ]);
    const page: Paged<TenantRow> = p;
    rows.value = page.items;
    total.value = page.total;
    summary.value = s;
  } finally {
    loading.value = false;
  }
}
onMounted(load);

function onSearch() {
  query.page = 1;
  load();
}

/** 分段筛选：点击即查询（不再需要单独的「查询」按钮） */
const segOptions = computed(() => [
  { value: "", label: "全部", count: summary.value.total },
  { value: "active", label: "启用", count: summary.value.active },
  { value: "pending", label: "待开通", count: summary.value.pending },
  { value: "suspended", label: "已停用", count: summary.value.suspended },
  { value: "expired", label: "已到期", count: summary.value.expired },
  { value: "archived", label: "已归档", count: summary.value.archived },
]);

function pickStatus(v: string) {
  if (query.status === v) return;
  query.status = v;
  query.page = 1;
  load();
}

/** 首列身份色块：按名称散列取色（tint 类定义在 theme.css） */
function tintOf(name: string) {
  let h = 0;
  for (const ch of name) h = (h * 31 + ch.charCodeAt(0)) % 997;
  return `tint-${h % 6}`;
}

/* ---------- 状态操作 ---------- */

const statusText: Record<TenantStatus, string> = {
  pending: "待开通", active: "启用", suspended: "已停用", expired: "已到期", archived: "已归档",
};
const statusTag: Record<TenantStatus, "success" | "warning" | "info" | "danger"> = {
  pending: "warning", active: "success", suspended: "warning", expired: "danger", archived: "info",
};

async function onToggleStatus(row: TenantRow) {
  const next: TenantStatus = row.status === "active" ? "suspended" : "active";
  const action = next === "active" ? "启用" : "停用";
  await ElMessageBox.confirm(`确定${action}租户「${row.name}」？${next === "suspended" ? "停用后该租户所有用户立即无法访问。" : ""}`, `${action}租户`, {
    type: next === "suspended" ? "warning" : "info",
    confirmButtonText: action,
  }).catch(() => Promise.reject());
  await setTenantStatus(row.id, next);
  ElMessage.success(`已${action}`);
  load();
}
async function onArchive(row: TenantRow) {
  await ElMessageBox.confirm(`归档后「${row.name}」的数据保留但不可恢复为活跃状态，确定继续？`, "归档租户", {
    type: "warning",
    confirmButtonText: "归档",
    confirmButtonClass: "el-button--danger",
  }).catch(() => Promise.reject());
  await archiveTenant(row.id);
  ElMessage.success("已归档");
  load();
}

function onCommand(cmd: string, row: TenantRow) {
  if (cmd === "toggle") onToggleStatus(row).catch(() => undefined);
  else if (cmd === "archive") onArchive(row).catch(() => undefined);
}

function openDetail(row: TenantRow) {
  router.push({ name: "tenant-detail", params: { id: row.id } });
}

/* ---------- 开通租户 ---------- */

const plans = ref<Plan[]>([]);
const dialog = ref(false);
const submitLoading = ref(false);
const formRef = ref<FormInstance>();
const form = reactive({
  name: "",
  code: "",
  subdomain: "",
  plan_code: "",
  timezone: "Asia/Shanghai",
  locale: "zh-CN",
  contact_name: "",
  contact_phone: "",
  contact_email: "",
  admin_email: "",
  admin_display_name: "",
});
const rules: FormRules = {
  name: [{ required: true, message: "请输入租户名称", trigger: "blur" }],
  code: [
    { required: true, message: "请输入租户标识", trigger: "blur" },
    { pattern: /^[a-z0-9][a-z0-9-]{1,63}$/, message: "小写字母或数字开头，可含 -", trigger: "blur" },
  ],
  subdomain: [{ pattern: /^[a-z][a-z0-9-]{1,30}$/, message: "小写字母开头，可含数字与 -", trigger: "blur" }],
  plan_code: [{ required: true, message: "请选择套餐", trigger: "change" }],
  admin_email: [
    { required: true, message: "请输入管理员邮箱", trigger: "blur" },
    { type: "email", message: "邮箱格式不正确", trigger: "blur" },
  ],
};

/** 创建成功后一次性展示初始密码（readme 契约：仅返回一次） */
const created = ref<{ name: string; subdomain?: string; password: string } | null>(null);
const createdDialog = computed({
  get: () => created.value !== null,
  set: (v: boolean) => { if (!v) created.value = null; },
});

async function openCreate() {
  dialog.value = true;
  if (!plans.value.length) plans.value = await listPlans();
}

function resetForm() {
  Object.assign(form, {
    name: "", code: "", subdomain: "", plan_code: "",
    timezone: "Asia/Shanghai", locale: "zh-CN",
    contact_name: "", contact_phone: "", contact_email: "",
    admin_email: "", admin_display_name: "",
  });
  formRef.value?.clearValidate();
}

async function onCreate() {
  const ok = await formRef.value?.validate().catch(() => false);
  if (!ok) return;
  submitLoading.value = true;
  try {
    const r = await createTenant({
      name: form.name.trim(),
      code: form.code.trim(),
      subdomain: form.subdomain.trim() || undefined,
      plan_code: form.plan_code,
      timezone: form.timezone,
      locale: form.locale,
      contact_name: form.contact_name.trim() || undefined,
      contact_phone: form.contact_phone.trim() || undefined,
      contact_email: form.contact_email.trim() || undefined,
      admin_email: form.admin_email.trim(),
      admin_display_name: form.admin_display_name.trim() || undefined,
    });
    dialog.value = false;
    created.value = { name: r.tenant.name, subdomain: r.tenant.subdomain, password: r.initial_password };
    resetForm();
    load();
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : "创建失败");
  } finally {
    submitLoading.value = false;
  }
}

async function copyPassword() {
  if (!created.value) return;
  await navigator.clipboard.writeText(created.value.password).catch(() => undefined);
  ElMessage.success("已复制到剪贴板");
}
</script>

<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1 class="page-title">租户管理</h1>
        <p class="page-sub">共 {{ summary.total }} 个租户 · {{ summary.active }} 个活跃</p>
      </div>
      <el-button
        v-if="auth.can(PERM.tenantCreate)"
        v-permission="PERM.tenantCreate"
        type="primary"
        :icon="Plus"
        @click="openCreate"
      >开通租户</el-button>
    </div>

    <div class="seg-bar">
      <button
        v-for="s in segOptions"
        :key="s.value"
        type="button"
        class="seg"
        :class="{ active: query.status === s.value }"
        @click="pickStatus(s.value)"
      >
        {{ s.label }}<span class="seg-count num">{{ s.count }}</span>
      </button>
    </div>

    <el-card shadow="never" class="table-card">
      <div class="card-toolbar">
        <el-input
          v-model="query.keyword"
          placeholder="搜索名称 / 标识 / 子域名"
          :prefix-icon="Search"
          clearable
          class="search-input"
          @clear="onSearch"
          @keyup.enter="onSearch"
        />
        <span v-if="query.keyword" class="toolbar-hint">筛选出 {{ total }} 条</span>
        <div class="toolbar-spacer" />
        <el-button text :icon="Refresh" title="刷新" @click="load" />
      </div>

      <el-table
        v-loading="loading"
        :data="rows"
        style="width: 100%"
        row-class-name="row-clickable"
        @row-click="openDetail"
      >
        <el-table-column label="租户" min-width="220">
          <template #default="{ row }">
            <div class="tenant-cell">
              <div class="avatar" :class="tintOf(row.name)">{{ row.name.slice(0, 1) }}</div>
              <div class="tenant-meta">
                <p class="t-name">{{ row.name }}</p>
                <p class="t-sub">{{ row.subdomain ? `${row.subdomain}.example.com` : row.code }}</p>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="套餐" width="110">
          <template #default="{ row }">
            <el-tag size="small" effect="plain">{{ row.plan_name || row.plan_code }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="110">
          <template #default="{ row }">
            <el-tag :type="statusTag[row.status as TenantStatus]" size="small" class="status-pill">{{ statusText[row.status as TenantStatus] }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="到期时间" width="140">
          <template #default="{ row }">
            <el-tooltip v-if="row.expire_at" :content="fmtDateTime(row.expire_at)" placement="top">
              <span class="t-sub time-cell">{{ untilNow(row.expire_at) }}</span>
            </el-tooltip>
            <span v-else class="t-sub">长期有效</span>
          </template>
        </el-table-column>
        <el-table-column label="创建时间" width="130">
          <template #default="{ row }">
            <el-tooltip :content="fmtDateTime(row.created_at)" placement="top">
              <span class="t-sub time-cell">{{ fromNow(row.created_at) }}</span>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="130" align="right">
          <template #default="{ row }">
            <div class="row-actions" @click.stop>
              <el-button text type="primary" size="small" @click="openDetail(row as TenantRow)">详情</el-button>
              <el-dropdown trigger="click" @command="(c: string) => onCommand(c, row as TenantRow)">
                <el-button text size="small" :icon="MoreFilled" />
                <template #dropdown>
                  <el-dropdown-menu>
                    <el-dropdown-item
                      v-if="auth.can(PERM.tenantUpdate)"
                      command="toggle"
                      :disabled="row.status === 'archived'"
                    >{{ row.status === "active" ? "停用租户" : "启用租户" }}</el-dropdown-item>
                    <el-dropdown-item
                      v-if="auth.can(PERM.tenantDelete)"
                      command="archive"
                      divided
                      :disabled="row.status === 'archived'"
                    >归档租户</el-dropdown-item>
                  </el-dropdown-menu>
                </template>
              </el-dropdown>
            </div>
          </template>
        </el-table-column>
        <template #empty>
          <div class="empty">
            <p class="empty-title">{{ query.keyword || query.status ? "没有符合条件的租户" : "还没有租户" }}</p>
            <p class="empty-sub">{{ query.keyword || query.status ? "换个关键词或状态试试" : "开通第一个租户后，它会出现在这里" }}</p>
            <el-button v-if="!query.keyword && !query.status" type="primary" size="small" :icon="Plus" @click="openCreate">开通租户</el-button>
          </div>
        </template>
      </el-table>

      <div class="table-foot">
        <span class="foot-total">共 {{ total }} 条</span>
        <el-pagination
          v-model:current-page="query.page"
          v-model:page-size="query.size"
          :total="total"
          :page-sizes="[10, 20, 50]"
          layout="sizes, prev, pager, next"
          @current-change="load"
          @size-change="onSearch"
        />
      </div>
    </el-card>

    <!-- 开通租户 -->
    <el-dialog v-model="dialog" title="开通租户" width="560px" :close-on-click-modal="false">
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
        <el-form-item label="租户名称" prop="name">
          <el-input v-model="form.name" placeholder="如：Acme 科技" />
        </el-form-item>
        <el-form-item label="租户标识" prop="code">
          <el-input v-model="form.code" placeholder="如：acme（唯一，创建后不可改）" />
        </el-form-item>
        <el-form-item label="子域名（可选）" prop="subdomain">
          <el-input v-model="form.subdomain" placeholder="如：acme">
            <template #append>.example.com</template>
          </el-input>
        </el-form-item>
        <el-form-item label="套餐" prop="plan_code">
          <el-select v-model="form.plan_code" placeholder="选择套餐" style="width: 100%">
            <el-option v-for="p in plans" :key="p.code" :value="p.code" :label="p.name" />
          </el-select>
        </el-form-item>
        <el-row :gutter="12">
          <el-col :span="12">
            <el-form-item label="时区" prop="timezone">
              <el-input v-model="form.timezone" placeholder="Asia/Shanghai" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="语言" prop="locale">
              <el-input v-model="form.locale" placeholder="zh-CN" />
            </el-form-item>
          </el-col>
        </el-row>
        <el-form-item label="管理员邮箱" prop="admin_email">
          <el-input v-model="form.admin_email" placeholder="admin@example.com" />
        </el-form-item>
        <el-form-item label="管理员姓名（可选）" prop="admin_display_name">
          <el-input v-model="form.admin_display_name" placeholder="管理员" />
        </el-form-item>
        <el-row :gutter="12">
          <el-col :span="12">
            <el-form-item label="联系人（可选）" prop="contact_name">
              <el-input v-model="form.contact_name" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="联系电话（可选）" prop="contact_phone">
              <el-input v-model="form.contact_phone" />
            </el-form-item>
          </el-col>
        </el-row>
        <el-form-item label="联系邮箱（可选）" prop="contact_email">
          <el-input v-model="form.contact_email" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialog = false">取消</el-button>
        <el-button type="primary" :loading="submitLoading" @click="onCreate">开通</el-button>
      </template>
    </el-dialog>

    <!-- 初始密码一次性展示 -->
    <el-dialog v-model="createdDialog" title="租户已开通" width="460px" :close-on-click-modal="false">
      <div v-if="created">
        <el-result icon="success" title="开通成功" :sub-title="created.subdomain ? `${created.name} · ${created.subdomain}.example.com` : created.name" />
        <el-alert type="warning" :closable="false" show-icon>
          <p>管理员初始密码（<b>仅显示这一次</b>，请立即复制保存）：</p>
        </el-alert>
        <div class="password-box num">{{ created.password }}</div>
      </div>
      <template #footer>
        <el-button @click="copyPassword">复制密码</el-button>
        <el-button type="primary" @click="created = null">我已保存</el-button>
      </template>
    </el-dialog>
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

/* 分段筛选 */
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
.seg-count {
  margin-left: 5px;
  font-size: 12px;
  opacity: 0.75;
}

/* 表格卡片 */
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
.search-input {
  width: 260px;
}
.toolbar-hint {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.toolbar-spacer {
  flex: 1;
}

.tenant-cell {
  display: flex;
  align-items: center;
  gap: 10px;
}
.avatar {
  width: 30px;
  height: 30px;
  border-radius: 8px;
  font-size: 12px;
  font-weight: 500;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.tenant-meta {
  min-width: 0;
}
.t-name {
  margin: 0;
  font-size: 13px;
  font-weight: 500;
  color: var(--el-text-color-primary);
}
.t-sub {
  margin: 1px 0 0;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.time-cell {
  cursor: default;
}
.row-actions {
  display: inline-flex;
  align-items: center;
  gap: 2px;
}
:deep(.row-clickable) {
  cursor: pointer;
}

.table-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
}
.foot-total {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.empty {
  padding: 24px 0 16px;
  text-align: center;
}
.empty-title {
  margin: 0;
  font-size: 14px;
  color: var(--el-text-color-primary);
}
.empty-sub {
  margin: 6px 0 12px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
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
