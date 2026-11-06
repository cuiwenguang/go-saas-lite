<script setup lang="ts">
import { useAuthStore } from "@/shared/auth/store";
import LoginShell from "@/shared/layout/LoginShell.vue";
import type { FormInstance, FormRules } from "element-plus";

const auth = useAuthStore();
const router = useRouter();
const route = useRoute();

const form = reactive({ email: "owner@acme.com", password: "" });
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

async function onSubmit() {
  const ok = await formRef.value?.validate().catch(() => false);
  if (!ok) return;
  loading.value = true;
  try {
    await auth.tenantLogin(form.email, form.password);
    router.push(String(route.query.redirect || "/app/dashboard"));
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : "登录失败");
  } finally {
    loading.value = false;
  }
}
</script>

<template>
  <LoginShell
    product-name="租户控制台"
    tagline="Acme 科技 · go-saas-lite"
    :bullets="['成员、角色与权限一目了然', 'API Key 与 MCP 工具集中管理', '配额用量实时可见']"
    footer="acme.example.com · 登录即代表同意服务条款"
  >
    <div class="login-card">
      <h2 class="login-title">登录 Acme 科技</h2>
      <p class="login-sub">当前访问的租户由子域名 acme.example.com 自动识别</p>

      <el-alert type="info" :closable="false" class="demo-tip">
        <p>演示账号：<b>owner@acme.com</b> · 密码任意（≥ 6 位）</p>
      </el-alert>

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
.demo-tip {
  margin-bottom: 20px;
}
.login-btn {
  width: 100%;
  margin-top: 4px;
}
</style>
