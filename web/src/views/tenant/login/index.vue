<script setup lang="ts">
import { useAuthStore } from "@/shared/auth/store";
import { changeTenantPassword } from "@/shared/api/auth";
import LoginShell from "@/shared/layout/LoginShell.vue";
import type { FormInstance, FormRules } from "element-plus";

const auth = useAuthStore();
const router = useRouter();
const route = useRoute();

/* 登录是全局的：邮箱全局唯一，登录后才知道用户属于哪个租户，不需要用户选择或填写租户 */
const form = reactive({ email: "", password: "" });
const rules: FormRules = {
  email: [
    { required: true, message: "请输入邮箱", trigger: "blur" },
    { type: "email", message: "邮箱格式不正确", trigger: "blur" },
  ],
  password: [
    { required: true, message: "请输入密码", trigger: "blur" },
    { min: 6, message: "密码至少 6 位", trigger: "blur" },
  ],
};
const formRef = ref<FormInstance>();
const loading = ref(false);

/* 初始密码首次登录必须改密：改完服务端会清掉刷新令牌，需要重新登录 */
const passwordDialog = ref(false);
const pwdForm = reactive({ next: "", confirm: "" });
const pwdLoading = ref(false);

async function onSubmit() {
  const ok = await formRef.value?.validate().catch(() => false);
  if (!ok) return;
  loading.value = true;
  try {
    const mustChange = await auth.tenantLogin(form.email, form.password);
    if (mustChange) {
      passwordDialog.value = true;

      return;
    }
    router.push(String(route.query.redirect || "/app/dashboard"));
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : "登录失败");
  } finally {
    loading.value = false;
  }
}

async function onChangePassword() {
  if (pwdForm.next.length < 8) {
    ElMessage.warning("新密码至少 8 位");

    return;
  }
  if (pwdForm.next !== pwdForm.confirm) {
    ElMessage.warning("两次输入的新密码不一致");

    return;
  }

  pwdLoading.value = true;
  try {
    await changeTenantPassword(form.password, pwdForm.next);
    ElMessage.success("密码已更新，请用新密码重新登录");
    passwordDialog.value = false;
    auth.logout();
    form.password = "";
    pwdForm.next = "";
    pwdForm.confirm = "";
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : "修改失败");
  } finally {
    pwdLoading.value = false;
  }
}
</script>

<template>
  <LoginShell
    product-name="租户控制台"
    tagline="go-saas-lite · 多租户管理"
    :bullets="['成员、角色与权限一目了然', 'API Key 与 MCP 工具集中管理', '配额用量实时可见']"
    footer="登录即代表同意服务条款"
  >
    <div class="login-card">
      <h2 class="login-title">租户控制台</h2>
      <p class="login-sub">使用邮箱与密码登录，系统会自动进入你所属的租户</p>

      <el-form ref="formRef" :model="form" :rules="rules" label-position="top" size="large" @keyup.enter="onSubmit">
        <el-form-item label="邮箱" prop="email">
          <el-input v-model="form.email" placeholder="name@acme.com" autocomplete="username" />
        </el-form-item>
        <el-form-item label="密码" prop="password">
          <el-input v-model="form.password" type="password" placeholder="密码" show-password autocomplete="current-password" />
        </el-form-item>
        <el-button type="primary" size="large" class="login-btn" :loading="loading" @click="onSubmit">登 录</el-button>
      </el-form>
    </div>

    <el-dialog v-model="passwordDialog" title="首次登录需修改密码" width="420px" :close-on-click-modal="false">
      <el-alert type="warning" :closable="false" show-icon style="margin-bottom: 12px">
        <p>当前使用的是管理员下发的初始密码，修改后才能进入控制台。</p>
      </el-alert>
      <el-form label-position="top">
        <el-form-item label="新密码">
          <el-input v-model="pwdForm.next" type="password" show-password placeholder="至少 8 位" />
        </el-form-item>
        <el-form-item label="确认新密码">
          <el-input v-model="pwdForm.confirm" type="password" show-password placeholder="再输一次" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button :loading="pwdLoading" type="primary" @click="onChangePassword">保存并重新登录</el-button>
      </template>
    </el-dialog>
  </LoginShell>
</template>

<style scoped>
.login-card {
  width: 380px;
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 14px;
  padding: 32px 36px;
}
.login-title {
  margin: 0;
  font-size: 20px;
  font-weight: 500;
}
.login-sub {
  margin: 8px 0 20px;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}
.login-btn {
  width: 100%;
  margin-top: 4px;
}
</style>
