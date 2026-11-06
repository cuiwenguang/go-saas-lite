# go-saas-lite 项目长期记忆

## 前端架构定案（2026-11-06，详见 CONVENTIONS-FRONTEND.md）

- 前端规范已同步为 skill：`.workbuddy/skills/go-saas-frontend/`（SKILL.md + references/ui-patterns.md），项目级共享；规范唯一源仍是根目录 `CONVENTIONS-FRONTEND.md`，skill 只是加载入口。

- 营销站 = Go 模板（`internal/web/`，Tailwind CLI 产单 CSS embed）；控制台 = `web/` 单 Vue 项目双入口（admin.html/tenant.html），Element Plus 按需 + axios + pinia + vxe-table + echarts，控制台禁 Tailwind、禁第二 UI 库、禁 workspace（升级触发条件见规范 §9）。
- 认证：access token 仅内存（pinia），refresh 走 httpOnly cookie；前端权限只认 `/me` 返回的权限点字符串（perms.ts 常量），禁角色字符串判断。
- URL/类型集中在 `web/src/shared/api/`（未来由 api/openapi.yaml 生成）；请求唯一通道 shared/http，业务代码禁裸 URL/绝对地址。
- 小程序预留：uni-app(Vue3)，仅共享逻辑层（http/auth 加适配器），UI 不复用。
- 后端 P0 前置：/auth/refresh、/me(含 permissions)、/public/signup+查重、/public/plans。

## 已知坑（勿重复踩）

- Go embed 空目录编译失败：dist 下必须提交 .gitkeep。
- NoRoute 不得无差别回 index.html：/api/* 未命中必须 JSON 404。
- Tailwind v4 不会自动扫 Go 模板：input.css 需 `@source "../../internal/web/templates/**/*.html"`。
- vxe-table Pro 付费红线：区域框选/Excel 复制粘贴/聚合汇总/查找替换/内置图表不引入。
- vite.config 双入口需 `base:"./"`，路由前缀运行时判断（/admin/ 或 /app/）。
- vxe-table v4.7+ 拆为 vxe-table + vxe-pc-ui 两个包，需同时安装。
