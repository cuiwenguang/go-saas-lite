# 当前实现基线：视觉与交互模式

> 本文件只描述 `web/` 已实现的设计语言，供新增页面 / 布局 / 交互时对齐。
> 规范条款以仓库根目录 `CONVENTIONS-FRONTEND.md` 为准；本文件是它的视觉补充，不引入新规则。

## 1. 布局骨架（AppShell / SideNav / Topbar）

### AppShell

- `100vh`，`el-container` 嵌套；
- 侧栏宽度 224px，折叠 64px，宽度动画 0.2s；
- header 高度 56px，**透明背景、无边框**，浮在页面底色上（内容白卡形成层次）；
- 侧栏右边框通高 1px；
- 路由切换：内容 8px 上移 + 180ms 淡入（`<transition name="page">`）。

### SideNav（三段式）

```
┌──────────────┐
│ Logo / 品牌   │  56px 高，下沿 1px 分割线
├──────────────┤
│ 导航菜单      │  flex:1，可滚动
│              │
├──────────────┤
│ 用户区        │  上沿 1px 分割线，贴底
└──────────────┘
```

- Logo 区与 Topbar 等高 56px，logo 方块 34px 垂直居中；
- 用户区：头像 32px + 名称 + 角色 / 套餐 + 下拉箭头；
  - 点击展开「账号设置 / 退出登录」；
  - 折叠后仅保留居中头像（中心 x=32）；
- 菜单项折叠态去水平 margin，64px 宽居中显示；
- 菜单由路由 meta + 权限点过滤生成，不单独维护菜单配置。

### Topbar

- 左侧：折叠按钮（34px 圆角）→ 1px 分隔线 → 面包屑；
  - 面包屑完整层级：「平台管理 / 当前页面」或「租户控制台 / 当前页面」；
  - 替代单独大标题，折叠按钮旁即可看到位置；
- 右侧 QuickActions（按端区分）：
  - **平台侧**：皮肤（亮/暗下拉）+ 消息（平台视角通知）+ AI（抽屉助手）；
  - **租户侧**：皮肤 + 消息（租户视角通知），**不显示 AI**；
- 请求进度条：视口顶端 fixed 2px 主色细线，`utils/progress.ts` 并发计数，最后一个完成才收尾。

## 2. 列表页工作区模板（以租户管理为基准）

1. **页面头成组**：标题 + 副标题（如「共 8 个 · 6 个活跃」）+ 主操作按钮上移到头部右侧；
2. **状态筛选前置**：分段 chips 带计数（全部 8 / 启用 6 / 已停用 1 / 已归档 1），点击即查询，去掉单独「查询」按钮；
3. **卡片内聚**：搜索 / 刷新收进表格卡头，不再散落在页面顶部；
4. **首列身份锚点**：首字母色块（按名称散列取色，`tint-0~5`），名称 + 域名两行；
5. **操作列收敛**：常用「详情」留外面，停用 / 启用 / 归档收进 `⋯` 下拉；
6. **行点击直达详情**；
7. **时间语义化**：`fromNow`（如「3 天前」），hover 出完整时间；
8. **空态带引导按钮**，不是裸「暂无数据」。

## 3. 组件视觉语言

### StatCard（编辑级排版）

- 30px 大数字（`letter-spacing: -0.5`），标签 12px 带 14px 小图标；
- 配额进度：自绘 4px 细线 + 右侧百分比数字（danger / warning / brand 三色）；
- 数字滚动：rAF 600ms ease-out，仅 number 类型；
- 数字列 `tabular-nums`，ID / 金额不抖动。

### 状态标签

- 统一软底色圆点 pill（`.status-pill`）：
  - active = 绿点绿字；
  - suspended = 琥珀点；
  - archived = 灰点；
- 不用 Element 默认实心 tag。

### 表格

- 字号 13px，行高收紧一档；
- 首列身份色块；
- 行点击进详情；
- 危险操作收进更多菜单。

### 登录页

- 左右分栏：左侧品牌区（logo + slogan + 渐变或纯色块），右侧表单；
- 租户侧带子域名识别提示；
- 不做深色侧边栏。

## 4. 主题与暗色

- `theme.css` 是全局唯一 Element 变量覆盖处：
  - 品牌色双 token：`--brand`（logo 色 `#534ab7`，用于 logo/图表/强调文字/大面积品牌区）+ `--brand-strong`（深一档 `#453c9e`，动作实底，与 logo 拉开层次）；`--el-color-primary*` 派生自 `--brand-strong`，并同步覆盖 `--el-color-primary-rgb`（否则残留 Element 默认蓝）；
  - 圆角 8px / 6px；
  - 边框弱化、表头透明、行 hover 浅灰；
  - 字体 Inter + PingFang SC；
- 暗色模式：`html.dark` 切换（Element dark css-vars + theme.css 的 dark 块）；
- **dev 权重坑**：Element 样式按需运行时注入、排在 theme.css 之后，覆盖 Element 必须加 `html ` 前缀（变量用 `:root:root`），且 main.ts 先引 `element-plus/theme-chalk/base.css`——否则 dev 下主题回退 Element 默认蓝而 build 产物正常，容易漏检；
- 切换入口唯一：QuickActions 的皮肤菜单（`app.setDark`）；
- **不做深色侧边栏**（用户已明确否决）。

## 5. 消息与 AI（当前为原型）

- 消息中心：3 条 mock 通知 + 未读圆点 + 全部已读；
  - 平台侧通知：租户开通、存储告警、平台周报；
  - 租户侧通知：存储配额、新成员加入、API Key 过期；
- AI 助手：右侧抽屉 + mock 对话 + 快捷提问；
  - **仅平台侧显示**；
  - 当前为本地模拟回答，不接真实模型。

## 6. 请求与进度

- axios 唯一实例，请求自动注入 `Authorization: Bearer <token>`；
- 任何请求发出时，视口顶端出现 2px 主色进度条；
- 并发请求计数，最后一个完成才收尾；
- 401 处理对业务透明（挂起 → refresh → 重放）。

## 7. 常用工具

- `utils/format.ts`：`fmtNumber`、`fmtDateTime`、`fromNow`、`cssVar`；
- `utils/progress.ts`：请求进度条；
- `shared/components/Chart.vue`：echarts 按需注册，颜色从 CSS 变量读取；
- `shared/components/StatCard.vue`：编辑级统计卡。

## 8. 已知视觉边界

- 不做深色侧边栏；
- 不给控制台引入 Tailwind；
- 不用第二个 UI 库；
- 不做复杂表格自助设计器（需求出现时优先评估 vxe Pro 或服务端生成 PDF）。
