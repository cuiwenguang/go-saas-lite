# 前端开发规范（web/）

> 适用范围：`web/` 目录——单仓库、双入口（admin.html / tenant.html）的 Vue 工程。
> 不适用：`internal/web/`（营销站，Go 模板，遵循 Go 侧 CONVENTIONS.md）。
> 效力：与 CONVENTIONS.md（后端）同等；冲突时以本文档为准（前端范围内）。
> 写法原则：只收「工具能检查」或「评审一眼能判」的条款；做不到这两条的不写。

---

## 1. 技术栈与硬边界（已定，不再讨论）

| 项 | 选型 |
| --- | --- |
| 框架 | Vue 3.5+ / `<script setup lang="ts">` / TypeScript strict |
| 构建 | Vite 双入口（admin.html + tenant.html），`base: "./"`，路由前缀运行时判断 |
| UI | Element Plus（unplugin 按需引入）+ @element-plus/icons-vue |
| 表格 | vxe-table + vxe-pc-ui（重数据网格场景） |
| 图表 | echarts + vue-echarts（按需注册） |
| 请求 | axios（唯一实例，见 §4） |
| 状态 | pinia（仅 §7 允许的 store） |
| 路由 | vue-router 4，history 模式 |

硬约束（MUST NOT）：

1. **不引入第二个 UI 组件库**。新组件优先 Element Plus，缺失能力用 vxe-table / 自写（scoped + Element 变量）。
2. **控制台不引入 Tailwind**（营销站专属，避免 preflight 与 Element 冲突）。
3. **不整包注册 Element**（`app.use(ElementPlus)` 禁止），必须 unplugin-auto-import / unplugin-vue-components 按需。
4. **不上 monorepo / pnpm workspace**。升级触发条件见 §9，未触发前不讨论。
5. vxe-table **Pro 付费功能不引入**（单元格区域框选、Excel 复制粘贴、内置图表、聚合汇总、查找替换）。需求若踩线，先评估付费或后端实现。

## 2. 目录与入口

```
web/
├── admin.html            平台后台入口          → dist/admin/
├── tenant.html           租户控制台入口        → dist/tenant/
├── vite.config.ts
├── src/
│   ├── main.admin.ts     仅平台侧：注册 Element、挂 router.admin、挂 app store
│   ├── main.tenant.ts    仅租户侧
│   ├── router.admin.ts   /admin/* 路由表（views/admin/ 全部懒加载）
│   ├── router.tenant.ts  /app/*  路由表（views/tenant/ 全部懒加载）
│   ├── views/
│   │   ├── admin/        平台后台页面（每页面独立 chunk）
│   │   └── tenant/       租户控制台页面
│   └── shared/           跨入口共用，禁止 import views/**
│       ├── http/         axios 实例、拦截器、401 刷新队列
│       ├── api/          按资源分文件（tenant.ts、user.ts…），唯一 URL 出处
│       ├── auth/         登录态、token 内存存储、/me 缓存
│       ├── permission/   v-permission 指令、权限点常量、菜单过滤
│       ├── layout/       侧边栏 + 顶栏 + 面包屑（两入口共用外壳）
│       ├── styles/       theme.css（全局唯一 Element 变量覆盖处）
│       └── utils/
```

规则：

- **views/admin/ 只允许被 router.admin.ts（动态 import）引用**；views/tenant/ 同理。两侧页面互不 import。CI 用 grep 校验。
- **shared/ 禁止 import `../views/**`**（依赖方向单一：views → shared）。CI 用 eslint `no-restricted-imports` 或 grep 校验。
- 页面目录 = `views/<side>/<module>/index.vue`，其 composable、子组件同目录放置。
- 命名：`.vue` 组件文件 PascalCase；其余 `.ts` 文件 camelCase；目录 kebab-case 或单词小写。
- 路由：`name` 唯一且与页面目录同名；`path` kebab-case；权限需求写在 `meta.perms`。

## 3. 组件写法

- 唯一写法 `<script setup lang="ts">`。禁止 Options API、mixins。
- 一个 SFC 一个导出组件。
- `defineProps` / `defineEmits` 必须用类型声明，不写运行时对象声明。
- **组件内禁止发请求**：请求写在页面级 composable（`useTenantList` 等）或 shared。组件只接 props / emit。
- 表单校验用 Element 的 rules（vxe 编辑表格用其内置校验），不自造校验器。

## 4. 请求与契约（唯一通道）

- **所有请求必须走 `shared/http` 导出的实例**。禁止新建 axios 实例、禁止 `fetch`、禁止在业务代码出现裸 URL 字符串。
- **URL 与类型只允许定义在 `shared/api/`**，按资源一个文件。后端接口字段变更时，PR 必须同步改这里（后续由 openapi.yaml 生成替代，手写期视为过渡）。
- 响应信封统一 `{code, msg, data}`：http 层解包——`code == 0` 返回 `data`；否则抛 `AppError`（含 code/msg）。页面 catch 后决定 `ElMessage.error` 与否；**http 层不弹 toast**（防止一次请求多份重复提示）。
- **401 处理对业务透明**：http 层挂起请求 → 调 `/api/auth/refresh` → 重放原请求；刷新失败 → 清空登录态并跳登录页。业务代码禁止自行处理 401。
- **禁止绝对地址**：业务代码只写相对路径（`/api/...`）。开发期由 Vite proxy 转发到 `:8080`，生产同域。环境变量只允许 `VITE_` 前缀。

## 5. 认证与权限

- **access token 只存 pinia（内存）**。MUST NOT 写 localStorage / sessionStorage（XSS 面）。
- refresh token 在 httpOnly cookie，前端不触碰。
- 路由守卫两级：未登录 → 登录页；已登录无权限 → 403 页。守卫实现在 shared/permission。
- **按钮 / 菜单 / 操作显隐只认权限点**：`v-permission="'tenant.user:create'"`，权限点常量集中在 `shared/permission/perms.ts`。**禁止按角色字符串判断**（与后端「权限点必须是代码常量」同源）。
- 菜单由路由 meta + `/me` 的 `permissions` 过滤生成，不单独维护菜单配置。

## 6. 样式与主题

- **全局 Element 变量覆盖只出现在 `shared/styles/theme.css`**（含 `--brand` 主色与 color-mix 派生色阶）。其他文件出现 `--el-` 覆盖即为违规。
- 组件内微调 Element 内部样式：必须 `scoped` + `:deep()`，并注释原因。
- **禁止硬编码颜色**（除 theme.css 与 ECharts 主题注册处）：用 `var(--el-*)`、语义色 class 或 `var(--brand)`。
- 间距尺度只用 4 / 8 / 12 / 16 / 24 / 32。页面容器 padding 24，卡片间 16，表单控件间距 12。
- ECharts 颜色从 CSS 变量读取注入主题，不散落 hex。
- 暗色模式：`html.dark` 切换（Element dark css-vars + theme.css 的 dark 块），切换入口唯一。

## 7. 状态管理

- pinia store 初始仅允许三个：`auth`（登录态/用户/权限点）、`app`（布局：折叠/暗色）、`tenant`（当前租户上下文，仅 tenant 侧挂载）。新增 store 需在 PR 说明理由。
- **服务端数据不进 pinia**：列表/详情由页面 composable 持有，切换页面重新请求。禁止把接口响应写 localStorage 当缓存。

## 8. 质量门（CI）

按顺序，任一失败即拒绝合并：

1. `pnpm lint`（eslint + eslint-plugin-vue + @typescript-eslint，prettier 统一格式）
2. `vue-tsc --noEmit`
3. `pnpm test`（vitest：http 拦截器、auth、permission 菜单过滤必须有单测）
4. `pnpm build`（产出 `dist/admin`、`dist/tenant`）
5. Go 侧 `go build ./...`（embed 要求 dist 非空，需提交 `.gitkeep` 占位）

### PR 自检清单

- [ ] 新页面在正确的 side（views/admin 或 views/tenant），懒加载
- [ ] 请求走 shared/http，URL 定义在 shared/api
- [ ] 权限控制用 v-permission + perms.ts 常量，无角色字符串判断
- [ ] 无硬编码颜色；Element 变量改动只在 theme.css
- [ ] 无新增 store（或已说明理由）
- [ ] 无第二个 UI 库 / Tailwind / 全量注册
- [ ] 列表页顶部有统计卡（遵循 §6 间距尺度的列表页模板）
- [ ] 新增依赖已在 PR 描述登记用途

## 9. 已知边界与升级触发条件

以下不做，直到触发条件出现：

| 触发条件 | 动作 |
| --- | --- |
| 租户控制台需要独立白标部署 / 独立发版节奏 | 拆 pnpm workspace，`src/shared` 平移为 `packages/core` |
| 两侧需要不同依赖版本或不同 UI 库 | 同上 |
| 小程序立项（uni-app，Vue 3） | 新建独立工程；只复用逻辑层（http 适配器换 uni.request、auth 换 storage），UI 不复用 |
| 团队多人接手且需要企业功能（多标签页路由、i18n） | 评估引入对应能力，单点实现不引整套脚手架 |

## 10. 与后端的接缝（Go 侧职责，前端不越界）

- `dist/admin`、`dist/tenant` 由 Go embed 提供；SPA history fallback 由 Go NoRoute 处理，**`/api/*` 未命中必须返回 JSON 404**（不允许吐 index.html）。
- 登录态接口契约：`POST /api/auth/refresh`、`GET /api/{platform,tenant}/v1/me`（返回 `permissions: string[]`）。缺这两个接口前端不开发登录后的任何页面。
- 开发期：`vite dev` 的 `/api` proxy 指向 `127.0.0.1:8080`。
