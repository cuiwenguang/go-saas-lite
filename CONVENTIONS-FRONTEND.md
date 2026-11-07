# 前端开发规范（web/）

> 适用范围：`web/` 目录——单仓库、双入口的 Vue 工程。
> 不适用：`internal/portal/`（营销站，Go 模板，遵循 Go 侧 CONVENTIONS.md）。
> 效力：与 CONVENTIONS.md（后端）同等；冲突时以本文档为准（前端范围内）。
> 写法原则：只收「工具能检查」或「评审一眼能判」的条款；做不到这两条的不写。

---

## 1. 技术栈与硬边界（已定，不再讨论）

| 项 | 选型 |
| --- | --- |
| 框架 | Vue 3.5+ / `<script setup lang="ts">` / TypeScript strict |
| 构建 | Vite 双入口（`admin/index.html` + `tenant/index.html`），`base: "./"`，路由前缀运行时判断 |
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
6. **不做深色侧边栏**（用户已明确否决）。整体视觉走“轻量 / 透明 / 留白”路线。

## 2. 目录与入口

```
web/
├── admin/index.html      平台后台入口          → dist/admin/
├── tenant/index.html     租户控制台入口        → dist/tenant/
├── vite.config.ts
├── scripts/serve-dist.mjs  dist 预览服务（history fallback，模拟 Go NoRoute）
├── src/
│   ├── main.admin.ts     仅平台侧：注册 Element、挂 router.admin、挂 app store
│   ├── main.tenant.ts    仅租户侧
│   ├── router.admin.ts   /admin/* 路由表（views/admin/ 全部懒加载）
│   ├── router.tenant.ts  /app/*  路由表（views/tenant/ 全部懒加载）
│   ├── views/
│   │   ├── admin/        平台后台页面（每页面独立 chunk）
│   │   └── tenant/       租户控制台页面
│   └── shared/           跨入口共用，禁止 import views/**
│       ├── http/         axios 实例、拦截器、mock 适配器挂载
│       ├── api/          按资源分文件，唯一 URL 出处
│       ├── auth/         登录态、token 内存存储、/me 缓存
│       ├── permission/   v-permission 指令、权限点常量、菜单过滤
│       ├── layout/       AppShell / SideNav / Topbar / QuickActions / LoginShell
│       ├── components/   StatCard / Chart 等跨页面组件
│       ├── mock/         原型期数据层（adapter + routes + 内存 db）
│       ├── stores/       app（布局：折叠/暗色）
│       ├── styles/       theme.css（全局唯一 Element 变量覆盖处）
│       └── utils/        format / progress
```

规则：

- **views/admin/ 只允许被 router.admin.ts（动态 import）引用**；views/tenant/ 同理。两侧页面互不 import。CI 用 grep 校验。
- **shared/ 禁止 import `../views/**`**（依赖方向单一：views → shared）。CI 用 eslint `no-restricted-imports` 或 grep 校验。
- 页面目录 = `views/<side>/<module>/index.vue`，其 composable、子组件同目录放置。
- 命名：`.vue` 组件文件 PascalCase；其余 `.ts` 文件 camelCase；目录 kebab-case 或单词小写。
- 路由：`name` 唯一且与页面目录同名；`path` kebab-case；权限需求写在 `meta.perms`。
- **多入口 HTML 必须放在子目录**（`admin/index.html`、`tenant/index.html`），平铺在根目录会导致 Vite 输出到 `dist/` 根而非 `dist/admin/`、`dist/tenant/`。

## 3. 组件写法

- 唯一写法 `<script setup lang="ts">`。禁止 Options API、mixins。
- 一个 SFC 一个导出组件。
- `defineProps` / `defineEmits` 必须用类型声明，不写运行时对象声明。
- **组件内禁止发请求**：请求写在页面级 composable（`useTenantList` 等）或 shared。组件只接 props / emit。
- 表单校验用 Element 的 rules（vxe 编辑表格用其内置校验），不自造校验器。
- **`v-model` 不能绑定表达式**（如 `created !== null`），需绑定可写 `ref` / `computed`。

## 4. 请求、契约与 Mock

- **所有请求必须走 `shared/http` 导出的实例**。禁止新建 axios 实例、禁止 `fetch`、禁止在业务代码出现裸 URL 字符串。
- **URL 与类型只允许定义在 `shared/api/`**，按资源一个文件。后端接口字段变更时，PR 必须同步改这里（后续由 openapi.yaml 生成替代，手写期视为过渡）。
- 响应信封统一 `{code, msg, data}`：http 层解包——`code == 0` 返回 `data`；否则抛 `AppError`（含 code/msg）。页面 catch 后决定 `ElMessage.error` 与否；**http 层不弹 toast**（防止一次请求多份重复提示）。
- **401 处理对业务透明**：http 层挂起请求 → 调 `/api/auth/refresh` → 重放原请求；刷新失败 → 清空登录态并跳登录页。业务代码禁止自行处理 401。
- **禁止绝对地址**：业务代码只写相对路径（`/api/...`）。开发期由 Vite proxy 转发到 `:8080`，生产同域。环境变量只允许 `VITE_` 前缀。
- **原型期 mock 挂在 axios 自定义适配器上**（`shared/http/mock.ts` + `shared/mock/routes.ts` + `shared/mock/db.ts`）：
  - URL、字段、信封必须与 readme 契约一致，切换真实后端时只删 mock 层，页面零改动；
  - **动态路由要按“具体路径在前、`:id` 在后”排序**（如 `/tenants/summary` 必须排在 `/tenants/:id` 前）；
  - mock 数据只服务演示，不承载业务规则；
  - `VITE_USE_MOCK=false` 时走真实请求（当前默认开启 mock）。
- **请求发出时自动注入 `Authorization: Bearer <token>`**（`setTokenProvider` 注入，避免 http 反向依赖 auth store 造成循环 import）。

## 5. 认证与权限

- **access token 只存 pinia（内存）**。MUST NOT 写 localStorage / sessionStorage（XSS 面）。
- refresh token 在 httpOnly cookie，前端不触碰。
- 路由守卫两级：未登录 → 登录页；已登录无权限 → 403 页。守卫实现在 shared/permission。
- **按钮 / 菜单 / 操作显隐只认权限点**：`v-permission="'tenant.user:create'"`，权限点常量集中在 `shared/permission/perms.ts`。**禁止按角色字符串判断**（与后端「权限点必须是代码常量」同源）。
- 菜单由路由 meta + `/me` 的 `permissions` 过滤生成，不单独维护菜单配置。
- 平台与租户两套身份物理隔离；`side` 属性贯穿布局与路由，禁止混用。

## 6. 样式与主题

- **全局 Element 变量覆盖只出现在 `shared/styles/theme.css`**（含 `--brand` 主色与 color-mix 派生色阶）。其他文件出现 `--el-` 覆盖即为违规。
- **品牌色双 token（`--brand` / `--brand-strong`）**：`--brand` = logo 色（当前 `#534ab7`，与 favicon、`.tint-0` 一致），用于 logo、图表、强调文字、大面积品牌区；`--brand-strong`（深一档）= 动作实底（按钮、聊天气泡等），与 logo 拉开层次。Element 的 `--el-color-primary*` 全部派生自 `--brand-strong`，且必须同步覆盖 `--el-color-primary-rgb`（否则残留默认蓝）。改色只改 theme.css 这两个值。
- 组件内微调 Element 内部样式：必须 `scoped` + `:deep()`，并注释原因。
- **样式权重约定（dev 必需）**：按需引入下 Element 的样式在运行时注入，顺序排在 `theme.css` **之后**，同权重会被它盖掉（dev 下主题回退为 Element 默认蓝；build 产物恰好顺序正确，容易漏检）。因此：theme.css 覆盖 Element 的选择器必须带 `html ` 前缀，变量块用 `:root:root` / `html.dark:root`；main.ts 必须在 theme.css 之前显式 `import "element-plus/theme-chalk/base.css"`。CI 可加 grep 检查：theme.css 中不允许出现裸的 `.el-` 开头选择器。
- **禁止硬编码颜色**（除 theme.css 与 ECharts 主题注册处）：用 `var(--el-*)`、语义色 class 或 `var(--brand)`。
- 间距尺度只用 4 / 8 / 12 / 16 / 24 / 32。页面容器 padding 24，卡片间 16，表单控件间距 12。
- ECharts 颜色从 CSS 变量读取注入主题，不散落 hex。
- 暗色模式：`html.dark` 切换（Element dark css-vars + theme.css 的 dark 块），切换入口唯一（`app.setDark` / 皮肤菜单）。
- **body 需显式 `margin: 0; padding: 0`**（Element Plus 不带全局 reset，浏览器默认 8px 边距会一直存在）。
- 数字列用 `tabular-nums`（`.num` class）。

## 7. 状态管理

- pinia store 初始仅允许三个：`auth`（登录态/用户/权限点）、`app`（布局：折叠/暗色）、`tenant`（当前租户上下文，仅 tenant 侧挂载）。新增 store 需在 PR 说明理由。
- **服务端数据不进 pinia**：列表/详情由页面 composable 持有，切换页面重新请求。禁止把接口响应写 localStorage 当缓存。

## 8. 当前实现基线（布局与工作区）

- **AppShell**：`100vh`，侧栏 224px / 折叠 64px，透明 header（无背景无边框），路由切换 8px 上移淡入。
- **SideNav 三段式**：Logo / 导航 / 底部用户。Logo 下沿与用户区上沿各 1px 分割线；用户区含头像、名称、角色/套餐、账号设置与退出登录；折叠后仅保留居中头像。
- **Topbar 左侧**：折叠按钮 → 分隔线 →「平台管理 / 当前页面」或「租户控制台 / 当前页面」面包屑，替代单独大标题。
- **Topbar 右侧 QuickActions**：
  - 平台侧：皮肤（亮/暗下拉）、消息（3 条 mock 通知 + 全部已读）、AI（右侧抽屉助手 + mock 对话）；
  - 租户侧：皮肤、消息（租户视角通知），**不显示 AI**。
- **列表页工作区模板**（以租户管理为基准）：
  - 页面头成组（标题 + 副标题 + 主操作按钮上移）；
  - 状态筛选为带计数的分段 chips（点击即查询，不再单独“查询”按钮）；
  - 搜索 / 刷新收进表格卡头；
  - 首列身份色块（按名称散列取色）；
  - 常用操作保留，危险操作收进 `⋯` 菜单；
  - 行点击直达详情；
  - 时间语义化（`fromNow`，hover 出完整时间）；
  - 空态带引导按钮。
- **StatCard 编辑级排版**：30px 大数字、标签缩小带 14px 图标、自绘 4px 进度线带百分比文字（danger/warning/brand 三色）。
- **状态标签统一为软底色圆点 pill**（`.status-pill`），不用 Element 默认实心 tag。
- **微动效三件套**：
  1. 顶栏请求进度条（视口顶端 2px 主色细线，`utils/progress.ts`，并发计数，最后一个完成才收尾）；
  2. 路由过渡（8px 上移 + 180ms 淡入）；
  3. StatCard 数字滚动（rAF 600ms ease-out，仅 number 类型）。
- **登录页**：左右分栏（左侧品牌区 + 右侧表单），租户侧带子域名识别提示。

## 9. 质量门（CI）

按顺序，任一失败即拒绝合并：

1. `npm run lint`（eslint + eslint-plugin-vue + @typescript-eslint，prettier 统一格式）
2. `vue-tsc --noEmit`
3. `npm run test`（vitest：http 拦截器、auth、permission 菜单过滤必须有单测）
4. `npm run build`（产出 `dist/admin`、`dist/tenant`）
5. Go 侧 `go build ./...`（embed 要求 dist 非空，需提交 `.gitkeep` 占位）

### PR 自检清单

- [ ] 新页面在正确的 side（views/admin 或 views/tenant），懒加载
- [ ] 请求走 shared/http，URL 定义在 shared/api
- [ ] 权限控制用 v-permission + perms.ts 常量，无角色字符串判断
- [ ] 无硬编码颜色；Element 变量改动只在 theme.css
- [ ] 无新增 store（或已说明理由）
- [ ] 无第二个 UI 库 / Tailwind / 全量注册
- [ ] 列表页顶部有统计卡（遵循 §8 工作区模板）
- [ ] 新增依赖已在 PR 描述登记用途
- [ ] mock 路由顺序正确（具体路径在前、`:id` 在后）
- [ ] UI 改动后用无头浏览器验证过布局与交互（脚本用完即删，不进仓库）

## 10. 已知边界与升级触发条件

以下不做，直到触发条件出现：

| 触发条件 | 动作 |
| --- | --- |
| 租户控制台需要独立白标部署 / 独立发版节奏 | 拆 pnpm workspace，`src/shared` 平移为 `packages/core` |
| 两侧需要不同依赖版本或不同 UI 库 | 同上 |
| 小程序立项（uni-app，Vue 3） | 新建独立工程；只复用逻辑层（http 适配器换 uni.request、auth 换 storage），UI 不复用 |
| 团队多人接手且需要企业功能（多标签页路由、i18n） | 评估引入对应能力，单点实现不引整套脚手架 |

## 11. 与后端的接缝（Go 侧职责，前端不越界）

- `dist/admin`、`dist/tenant` 由 Go embed 提供；SPA history fallback 由 Go NoRoute 处理，**`/api/*` 未命中必须返回 JSON 404**（不允许吐 index.html）。
- 登录态接口契约：`POST /api/auth/refresh`、`GET /api/{platform,tenant}/v1/me`（返回 `permissions: string[]`）。缺这两个接口前端不开发登录后的任何页面。
- 开发期：`vite dev` 的 `/api` proxy 指向 `127.0.0.1:8080`。
- 原型期后端接口一行不动；数据全部由 mock 层提供，URL/字段对齐 readme 契约。

## 12. 已踩过的坑（勿重复踩）

1. `v-model` 绑定表达式（如 `created !== null`）会报错，改为可写 ref / computed。
2. Vite 多入口 HTML 必须放子目录（`admin/index.html`），否则产物平铺到 `dist/` 根。
3. vue-router 的 `createWebHistory(base)` 与路由表自带前缀**只能二选一**，否则登录后白屏且无报错（生产构建剥掉 vue-router 警告）。
4. el-table 插槽的 `row` 类型是 `DefaultRow`，调用页面函数前需 `row as XxxRow` 收窄。
5. Element Plus 不带全局 reset，`body` 默认 8px margin 必须显式归零。
6. axios 1.x 的 headers 是 AxiosHeaders 实例，mock 解析 `Authorization` 时优先用 `h.get(...)`。
7. Tailwind v4 的 `@source` 不会自动扫 Go 模板目录，营销站 CSS 需显式指向 `internal/web/templates/**`（控制台不用 Tailwind）。
8. Go embed 空目录编译失败，`dist/` 下必须提交 `.gitkeep`。
9. vxe-table v4.7+ 拆为 vxe-table + vxe-pc-ui 两个包，需同时安装。
10. 图表（echarts）只随 dashboard 页面懒加载，不要在入口全量注册。
