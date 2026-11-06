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
import { setUnauthorizedHandler, setTokenProvider } from "@/shared/http/client";
import { useAuthStore } from "@/shared/auth/store";
import { useAppStore } from "@/shared/stores/app";

/* 租户控制台入口（/app/）。与平台侧唯一的差别：路由表与登录接口，其余全部共享 */

const app = createApp(App);
app.use(createPinia());
setupPermission(app);

useAppStore().init();

/* token 注入接线：登录后所有请求自动带 Authorization */
setTokenProvider(() => useAuthStore().token);

setUnauthorizedHandler(() => {
  const auth = useAuthStore();
  auth.logout();
  ElMessage.error("登录已失效，请重新登录");
  router.push({ name: "login" });
});

app.use(router);
app.mount("#app");
