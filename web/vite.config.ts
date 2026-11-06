import { resolve } from "node:path";
import { defineConfig, type Plugin } from "vite";
import vue from "@vitejs/plugin-vue";
import AutoImport from "unplugin-auto-import/vite";
import Components from "unplugin-vue-components/vite";
import { ElementPlusResolver } from "unplugin-vue-components/resolvers";

/* 开发期 history fallback：/admin/* -> admin.html，/app/* -> tenant.html（生产由 Go NoRoute 承担） */
function spaFallback(): Plugin {
  return {
    name: "spa-fallback",
    configureServer(server) {
      server.middlewares.use((req, _res, next) => {
        const url = (req.url || "").split("?")[0];
        if (url === "/admin" || url.startsWith("/admin/")) req.url = "/admin/index.html";
        else if (url === "/app" || url.startsWith("/app/")) req.url = "/tenant/index.html";
        next();
      });
    },
  };
}

// 双入口：admin.html -> dist/admin/，tenant.html -> dist/tenant/
// base 用相对路径，两个入口共用 dist/assets；路由前缀运行时判断（见 router.*.ts）
// 开发期访问 /admin.html 或 /tenant.html（或直接 /admin /app，见下方 rewrite）
export default defineConfig({
  base: "./",
  plugins: [
    vue(),
    spaFallback(),
    AutoImport({
      imports: ["vue", "vue-router"],
      resolvers: [ElementPlusResolver()],
      dts: "src/auto-imports.d.ts",
    }),
    Components({
      resolvers: [ElementPlusResolver()],
      dts: "src/components.d.ts",
    }),
  ],
  resolve: {
    alias: { "@": resolve(__dirname, "src") },
  },
  build: {
    outDir: "dist",
    rollupOptions: {
      input: {
        admin: resolve(__dirname, "admin/index.html"),
        tenant: resolve(__dirname, "tenant/index.html"),
      },
    },
  },
  server: {
    port: 5173,
    proxy: {
      // 对接真实后端时（npm run dev:api），API 走代理；mock 模式不发起真实请求
      "/api": { target: "http://127.0.0.1:8080", changeOrigin: false },
    },
  },
});
