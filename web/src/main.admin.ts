import { createApp } from "vue";
import { createPinia } from "pinia";
/* 显式先引 Element 基线（变量 + reset）：按需引入时组件样式是运行时注入的，
 * 顺序会排到 theme.css 之后，导致主题被默认值覆盖（dev 下尤其明显） */
import "element-plus/theme-chalk/base.css";
import "element-plus/theme-chalk/dark/css-vars.css";
import "@/shared/styles/theme.css";
import App from "./App.vue";
import { router } from "./router.admin";
import { setupPermission } from "@/shared/permission/directive";
import { setRefreshHandler, setUnauthorizedHandler, setTokenProvider } from "@/shared/http/client";
import { refreshPlatformSession } from "@/shared/api/auth";
import { useAuthStore } from "@/shared/auth/store";
import { useAppStore } from "@/shared/stores/app";

/* 平台后台入口（/admin/）。请求拦截器的 401 回调在这里接线（CONVENTIONS-FRONTEND §4） */

const app = createApp(App);
app.use(createPinia());
setupPermission(app);

useAppStore().init();

/* token 注入接线：登录后所有请求自动带 Authorization */
setTokenProvider(() => useAuthStore().token);

/* 401 透明刷新接线：refresh 令牌在 httpOnly cookie，前端只收新的 access token */
setRefreshHandler(async () => {
  const session = await refreshPlatformSession();
  useAuthStore().setToken(session.access_token);
  return session.access_token;
});

setUnauthorizedHandler(() => {
  const auth = useAuthStore();
  auth.logout();
  ElMessage.error("登录已失效，请重新登录");
  router.push({ name: "login" });
});

app.use(router);
app.mount("#app");
