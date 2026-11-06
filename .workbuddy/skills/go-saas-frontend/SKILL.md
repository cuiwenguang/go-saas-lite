---
name: go-saas-frontend
description: go-saas-lite 控制台前端（web/ 双入口 Vue 工程）开发规范与实现模式。当用户要求新增、修改或评审 web/ 目录下的页面、布局、请求、mock、权限、主题、构建或 UI 交互时使用；不适用于营销站 internal/web/。
agent_created: true
---

# go-saas-lite 前端规范

## 目的

约束 `web/` 目录下的控制台前端开发，保证平台后台与租户控制台在双入口、请求契约、权限、主题和视觉语言上不漂移。

## 触发场景

- 新增 / 修改 / 评审 `web/src/**` 下任何页面、组件、布局或样式；
- 调整请求、mock、认证、权限、路由或构建配置；
- 讨论前端技术选型、目录结构或 UI 交互方案；
- 输出前端相关代码审查意见。

不适用：`internal/web/`（营销站，Go 模板）。

## 使用流程

1. **先读仓库根目录的 `CONVENTIONS-FRONTEND.md`**——它是唯一规范源，包含技术栈硬边界、目录规则、请求与 mock 契约、权限、样式、状态、质量门、当前实现基线和已踩坑清单。
2. 涉及**页面 / 布局 / 视觉 / 交互**时，再读 `references/ui-patterns.md`，按当前已实现的设计语言落地，不要自行发明风格。
3. 修改前先定位现有实现，优先复用 `shared/` 中的组件与工具，不要在页面内复制粘贴布局或请求逻辑。
4. 任何新增页面遵循 `views/<side>/<module>/index.vue` 结构，并挂到对应 router（`router.admin.ts` / `router.tenant.ts`）。
5. 完成后必须执行质量门：
   ```bash
   npm run build --prefix web
   web/node_modules/.bin/vue-tsc --noEmit -p web/tsconfig.json
   ```
6. UI 改动建议用无头浏览器验证布局与交互（本机可用 Edge + puppeteer-core）；验证脚本与临时依赖**用完即删，不进仓库**。
7. 如需本地演示，运行 `npm run preview --prefix web`（`scripts/serve-dist.mjs`，端口 4173，带 history fallback）。

## 高压线（摘要，详细见规范文档）

1. 不引入第二个 UI 库；控制台不引 Tailwind。
2. 不整包注册 Element Plus，必须按需引入。
3. 不上 monorepo / pnpm workspace（触发条件见规范 §10）。
4. vxe-table Pro 付费功能不引入。
5. 所有请求走 `shared/http`，URL 与类型只写在 `shared/api/`。
6. access token 只存内存 pinia；按钮 / 菜单显隐只认权限点，不认角色字符串。
7. Element 变量覆盖只允许在 `shared/styles/theme.css`；禁止硬编码颜色。
8. 不做深色侧边栏；视觉走“轻量 / 透明 / 留白”路线。
9. 原型期后端接口一行不动，数据全部走 mock 层，URL/字段对齐 readme 契约。
10. mock 动态路由按“具体路径在前、`:id` 在后”排序。

## 资源

- `references/ui-patterns.md`：当前已实现的视觉与交互模式（布局、工作区、动效、状态、消息与 AI）。
- 仓库根目录 `CONVENTIONS-FRONTEND.md`：唯一规范源，必须先读。

## 维护

- 规范或实现变更时，先更新 `CONVENTIONS-FRONTEND.md`，再同步本 skill 与 `references/ui-patterns.md`；
- 本 skill 为项目级（`.workbuddy/skills/go-saas-frontend/`），随仓库共享；
- 不要把临时验证脚本、截图或 `node_modules` 打进 skill。
