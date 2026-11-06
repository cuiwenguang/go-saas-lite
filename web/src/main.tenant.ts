import { createApp } from "vue";
import { createPinia } from "pinia";
/* 显式先引 Element 基线（变量 + reset）：按需引入时组件样式是运行时注入的，
 * 顺序会排到 theme.css 之后，导致主题被默认值覆盖（dev 下尤其明显） */
import "element-plus/theme-chalk/base.css";
import "element-plus/theme-chalk/dark/css-vars.css";
import "@/shared/styles/theme.css";
import App from "./App.vue";
import { router } from "./router.tenant";
import { setupPermission } from "@/shared/permission/directive";
import { setRefreshHandler, setTenantProvider, setUnauthorizedHandler, setTokenProvider } from "@/shared/http/client";
import { refreshTenantSession } from "@/shared/api/auth";
import { currentTenantKey } from "@/shared/tenant/context";
import { useAuthStore } from "@/shared/auth/store";
import { useAppStore } from "@/shared/stores/app";

/* 租户控制台入口（/app/）。与平台侧唯一的差别：路由表与登录接口，其余全部共享 */

const app = createApp(App);
app.use(createPinia());
setupPermission(app);

useAppStore().init();

/* token 注入接线：登录后所有请求自动带 Authorization */
setTokenProvider(() => useAuthStore().token);

/* 租户标识接线：子域名 / 本地记忆解析结果，登录前也要带上 */
setTenantProvider(() => currentTenantKey());

/* 401 透明刷新：租户 refresh 令牌在 httpOnly cookie 里，前端只管拿新的 access token */
setRefreshHandler(async () => {
  const session = await refreshTenantSession();
  const auth = useAuthStore();
  auth.setToken(session.access_token);

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
