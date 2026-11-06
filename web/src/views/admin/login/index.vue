<script setup lang="ts">
import { useAuthStore } from "@/shared/auth/store";
import LoginShell from "@/shared/layout/LoginShell.vue";
import type { FormInstance, FormRules } from "element-plus";

const auth = useAuthStore();
const router = useRouter();
const route = useRoute();

const form = reactive({ email: "admin@example.com", password: "" });
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
    await auth.platformLogin(form.email, form.password);
    router.push(String(route.query.redirect || "/admin/dashboard"));
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : "登录失败");
  } finally {
    loading.value = false;
  }
}
</script>

<template>
  <LoginShell
    product-name="go-saas-lite"
    tagline="多租户 SaaS 底座 · 平台管理控制台"
    :bullets="['租户开通与生命周期管理', '套餐与配额随时调整', '全链路可观测：日志、指标、链路']"
    footer="go-saas-lite · 平台管理"
  >
    <div class="login-card">
      <h2 class="login-title">登录平台管理</h2>
      <p class="login-sub">仅平台管理员可访问，与租户控制台相互隔离</p>

      <el-alert type="info" :closable="false" class="demo-tip">
        <p>演示账号：<b>admin@example.com</b> · 密码任意（≥ 6 位）</p>
      </el-alert>

      <el-form ref="formRef" :model="form" :rules="rules" label-position="top" size="large" @keyup.enter="onSubmit">
        <el-form-item label="邮箱" prop="email">
          <el-input v-model="form.email" placeholder="admin@example.com" autocomplete="username" />
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
