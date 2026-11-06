# 编码规范（已迁移）

> **本文件已废弃，规范正文迁移为 Agent Skill：**
> **`.github/skills/go-saas-lite-conventions/SKILL.md`**
>
> 开发时以 SKILL.md 为准；产品行为与边界仍以 `readme.md` 为准。
> 下方内容仅作历史参考，请勿在此追加规则。

---

## 1. `internal` 与 `pkg` 的归属

### 1.1 判定标准（一句话）

**业务 = 对外暴露能力的模块**（前端 UI 可调用的 HTTP 接口、MCP 端点）→ `internal`。
**基础设施 = 只被业务调用的机制**（数据库、缓存、配置、观测、迁移）→ `pkg`。

判据不是"我觉得它通用"，而是问一句：**这个包提供的是「能力」还是「机制」？**
- 能力（做什么）：有 HTTP / MCP 入口、有产品业务语义 → `internal`。
- 机制（怎么做）：被能力调用、可整体拷走到另一个项目不改代码就编译 → `pkg`。

硬约束：`pkg/*` 不得 import `internal/*`（CI 用 depguard 卡住）。

### 1.2 现有归属

| 包 | 归属 | 理由 |
|---|---|---|
| `apperr` `response` `obs` `redisx` | pkg | 与业务无关的通用机制 |
| `tenancy` `migrate` | pkg | 数据底座机制，不依赖任何业务模型 |
| `auth` `authz` `apikey` `oauth` `mcp` `quota` | internal | 对外暴露能力：鉴权 / 授权 / API Key / OAuth / MCP 端点 / 配额校验 |
| `tenantconf` `platform/tenant` `tenantaccount` `platformauth` `mcpgateway` `worker` | internal | 含租户、平台、工具等产品语义，对外暴露接口 |
| `app` `testutil` | internal | 本项目装配与测试工具 |

### 1.3 `pkg` 是否允许带 HTTP handler —— **禁止（P2，已定）**

`pkg` 只放机制，所有 HTTP / MCP 适配进 `internal`。能力（含协议入口）一律 `internal`，不允许"有的带、有的不带"。

---

## 2. 模块内部结构与分层

### 2.1 标准文件集

一个业务模块（`internal/<domain>/` 或其子包）固定这些文件，**按职责命名，不按层建目录**：

| 文件 | 职责 | 可否省略 |
|---|---|---|
| `handler.go` | 绑定、校验、统一响应；唯一知道 HTTP 的层 | 不可 |
| `service.go` | 业务规则、事务边界、跨组件编排；模块对外唯一入口 | 不可 |
| `model.go` | 表映射；命名用领域名词，**不带 `Row`/`Model` 后缀** | 无表可省 |
| `dto.go` | service 的入参 / 出参结构（`XxxInput` / `XxxOutput`） | 可选 |
| `perm.go` | 本模块权限点常量与登记 | 无权限点可省 |
| `store.go` / `source.go` | 读写适配（被 service 调用，handler 不得直接调用） | 可选 |
| `provisioner.go` / `worker.go` | 有独立生命周期的任务编排 | 可选 |
| `module.go` | fx 装配（§4 方案 A） | 必须 |

> 复杂 `service` 可以拆多个文件（`provisioner.go`、`sync.go`、`store.go`），
> 每个文件开头必须有一行注释说明它做什么；辅助函数一律小写（包内私有），不得被 handler 引用。

### 2.2 分层硬规则

```
handler  ──►  service  ──►  tenancy.DB / PlatformDB / Redis
```

- **MUST**：`handler` 只依赖 `service`、`response`、`apperr`、`authz.Authorizer`、绑定与校验。
- **MUST NOT**：`handler` 出现 `tenancy.DB`、`tenancy.PlatformDB`、`redis.Client`、`*gorm.DB`。
- **MUST NOT**：`handler` 写数据访问代码，哪怕只有一行（例如"登录时顺手查一下用户表"）。
- **MUST**：事务只在 `service` 开启（`tenancy.DB.Tx`），handler / store 不得 `Begin`/`Commit`。
- **MUST**：纯 CRUD 也保留 `service`，允许它只有转发——**豁免会让规则无法机器校验**。
- **MUST**：入参 / 出参结构体定义在 **service 包的 `dto.go`**，命名 `XxxInput` / `XxxOutput`；handler 直接绑定 `XxxInput`，不另建 `XxxRequest`。简单模块允许在 `service.go` 里就地声明。
- **MUST NOT**：handler 拥有独立的 dto 文件（`Request` / `Response` 结构放 handler 包会造成双向依赖）。

反例（现存）：

```go
// internal/tenantaccount/handler.go
func (h *Handler) login(c *gin.Context) {
    err := h.db.Read(ctx, func(tx tenancy.Tx) error { ... }) // handler 直接读库
}
```

正确形态：

```go
// handler 只做协议
result, err := h.service.Login(c.Request.Context(), loginInput)

// service 持有 tenancy.DB，负责事务与规则
func (s *Service) Login(ctx context.Context, in LoginInput) (*LoginResult, error)
```

### 2.3 组件命名白名单

`service` 之下的组件**只允许**以下命名，且**不得被 handler 直接引用**：

| 命名 | 用途 |
|---|---|
| `Service` | 模块唯一对外入口 |
| `Provisioner` / `Worker` / `Sync` | 有独立生命周期的任务 |
| `Store` | KV / 配置类读写 |
| `Source` | 只读数据源 |
| `Provider` | 外部能力的实现适配器（实现 `pkg` 里的接口） |
| `SessionStore` / `Broadcaster` 等 | 实现 `pkg` 已定义接口的适配器 |

> 结论：`tenantconf` 现在的 `Store` 必须补一层 `Service`（或整体改名 `Service`），否则等于 handler 直连数据层。

### 2.4 例外申请

违反 §2.2 的代码必须满足三条，否则评审不通过：

1. 文件内 `//nolint:layering // 理由`；
2. 理由写清"为什么放进 service 反而更糟"；
3. 在 PR 描述里单独列出。

---

## 3. 命名规范

### 3.1 Go 侧

| 对象 | 规则 | 示例 |
|---|---|---|
| 构造函数 | `NewXxx`，返回具体类型或接口；基础工具包单一入口用 `New` | `tenant.NewService`、`redisx.New` |
| 模块入口类型 | `Service` | `tenant.Service`、`apikey.Service` |
| 多职责入口 | 业务名，但由 `Service` 编排 | `Provisioner`、`Worker` |
| 表模型 | 领域名词，无 `Row`/`Model` 后缀 | `Role`、`OAuthClient`、`APIKey` |
| DTO 入参 / 出参 | `XxxInput` / `XxxOutput`，定义在 service 包 | `CreateTenantInput`、`LoginOutput` |
| HTTP 适配 | `Handler`，方法 `Register(group)` | `tenant.Handler` |
| 中间件 | 返回 `gin.HandlerFunc` | `auth.Guard`、`quota.Middleware` |
| 选项 | `Option` + `WithXxx` | `auth.WithAccessTTL` |
| 接口 | 在使用方定义，动词或 `-er` 结尾 | `mcp.Provider`、`tenancy.Tx` |

### 3.2 数据与配置

| 对象 | 规则 | 示例 |
|---|---|---|
| 表名 | 复数下划线；租户表必带 `tenant_id` | `tenant_configs`、`mcp_call_log` |
| 列名 | 下划线 | `provision_status` |
| 迁移文件 | `NNNNN_描述.sql`，组内递增 | `00006_oauth_grants.sql` |
| 配置字段 | 同时具备 `koanf` tag、yaml 项、默认值、校验 | `koanf:"platform_dsn"` |
| 权限点 | `obj:act`，obj 可点分层，动作禁止 `_` | `order:read`、`tenantconf:update` |
| 错误码 | 5 位分段；框架只占 1–4 段 | `20005` |
| Redis key | `tenant:{tid}:biz:...`，由框架拼接 | `tenant:{tid}:quota:qps:{sec}` |

### 3.3 测试命名

`Test<被测对象>_<行为>`，行为用中文描述更易读：

```go
func TestTenantService_开通后内置角色与管理员就绪(t *testing.T)
```

---

## 4. fx 装配（方案 A：模块自治，已定）

每个 `internal` 业务模块导出 `Module() fx.Option`，`internal/app` 只做编排。

### 4.1 规则

```go
// internal/platform/tenant/module.go
func Module() fx.Option {
    return fx.Module("platform.tenant",
        fx.Provide(NewService, NewHandler),
        fx.Invoke(registerRoutes),
    )
}

// internal/app/app.go
fx.Options(Core(cfg), tenant.Module(), tenantconf.Module(), ...)
```

- 需把 `Routes` 抽到 `internal/routes`，否则模块 import `internal/app` 会形成循环。
- 新增 8 个集中装配（不采用，仅备查）

> 曾考虑集中装配：所有 `fx.Provide` 与路由挂载在 `internal/app/app.go`。
> 已否决：readme 3.4 承诺了 `Module()`，且模块自治的依赖关系更易审。
> 历史集中注册代码在 §11 清单收敛后删除
- 与 readme 3.4 冲突，需要同步改 readme。
- 优点：依赖关系集中，一眼看全。

### 4.3 与方案无关的已确定项

- `*gorm.DB` 不出 `internal/app`（用包内类型 `tenantGorm` / `platformGorm` 包住）。
- `fx.Invoke` 只做注册与触发，不写业务。
- OnStop 逆序：先停 HTTP，再关依赖；迁移 checksum 校验必须在 HTTP OnStart 之前。
- 依赖方向：`cmd → internal/app → internal/*`，`pkg/*` 不得 import `internal/*`。

---

## 5. 数据访问

- **MUST**：只用 `tenancy.DB` / `tenancy.PlatformDB`，业务代码不出现 `*gorm.DB`。
- **MUST**：平台组表只能经 `PlatformDB`；新增平台表必须同步 `pkg/tenancy` 的 `platformTables`。
- **MUST**：原生 SQL 遵守 readme 3.2.1（单条 SELECT、`tenant_id` 显式、配隔离用例）。
- **MUST**：跨租户维护操作（如清理）用 `Tx.DeleteAll`，不得绕开 `tenancy.DB` 手写 SQL。
- **MUST NOT**：`pkg` 里 import `internal`（CI 用 depguard 卡住）。

---

## 6. 错误、响应、日志

- 领域错误一律 `apperr.New` / `apperr.Wrap`；`handler` 只调 `response.Fail`，不手写状态码。
- 业务代码禁止 `c.JSON(500, ...)`。
- 日志统一 `slog.InfoContext(ctx, ...)`；`tenant_id` / `user_id` / `request_id` / `trace_id` 由 `pkg/obs` 自动补全。
- 禁止在 `pkg` 里 `os.Exit`、直接读环境变量（配置由 `cmd` 注入）。

---

## 7. 配置

新增一个配置项的完整动作（缺一不可）：

1. struct 字段 + `koanf` tag；
2. `configs/config.yaml` 对应项（注释写明环境变量名）；
3. `DefaultConfig()` 默认值；
4. `validate()` 校验；
5. 至少一条 `LoadConfig` 绑定测试。

> 教训：早期漏了 `koanf` tag，`read_header_timeout_sec` 这类配置从未生效，只因默认值恰好相同而未暴露。

---

## 8. 迁移与表归属

- 两组目录：`migrations/platform`、`migrations/tenant`；版本表 `schema_migrations_platform` / `schema_migrations_tenant`。
- checksum 记在 `schema_migrations_checksum`；启动 `Verify` 失败即拒绝启动。
- 多进程并发迁移用 `pg_advisory_lock` 串行化。
- 新增表必须同时完成：写迁移、登记归属白名单（平台组）、补隔离用例（租户组）。

---

## 9. 测试

### 9.1 测试放哪里

| 类型 | 放哪里 | 示例 |
|---|---|---|
| 单元测试（纯内存 / 桩） | **与被测代码同目录**，`package xxx_test` | `internal/authz/authorizer_test.go` |
| 需要真实 PG / Redis 的集成测试 | **`internal/integration/`** | 隔离用例、OAuth 流程 |
| 装配 / 契约测试（路由、fx 图、生命周期） | `internal/app/` | `app_test.go`、`mcp_flow_test.go` |
| 夹具与断言库 | `internal/testutil/`，**只放夹具，不放用例** | `Harness`、`SeedRole`、`MustNotSeeWidget` |

`internal/testutil` 的定位是**测试夹具库**：造库、跑迁移、建租户、断言工具。
它不存放业务测试用例，也不需要"所有测试统一放这里"。

### 9.2 pkg 不得依赖 internal/testutil

`pkg/` 的生产代码不能 import `internal/`（§1）；同理，**`pkg` 的测试也不要 import `internal/testutil`**，否则 `pkg` 失去"整包拷走"属性。
需要真实库的 `pkg` 行为，统一由 `internal/integration` 覆盖。

### 9.3 必须遵守

- 隔离用例：不 mock `tenancy.DB`。
- 新增租户组表：**必须**补"B 租户读 / 改 / 删不到 A"用例。
- 新增 `Raw` SQL：**必须**补一条双租户用例。
- 数据库来源：`TEST_DATABASE_DSN` 优先；未设置时自动起 embedded-postgres；都不可用才 skip。
- 断言针对"自己的租户 / 记录"，不依赖全局计数（共享库会累积历史数据）。
- CI 必须跑：`gofmt -l`、`go vet`、`go test ./...`、`fx.ValidateApp`。

---

## 10. 评审清单

- [ ] 新包归属符合 §1，能通过"拷走能否编译"
- [ ] handler 无 DB / Redis / gorm
- [ ] service 是模块唯一对外入口，事务在 service
- [ ] 组件命名在 §2.3 白名单内
- [ ] 新配置具备 5 件套（§7）
- [ ] 新表：迁移 + 白名单 + 隔离用例
- [ ] 新权限点：代码常量 + 登记 + 内置角色
- [ ] 错误码分段正确，无裸 `c.JSON(500)`
- [ ] 测试通过，且不依赖全局计数
- [ ] 若偏离规范，已按 §2.4 登记例外

---

## 11. 存量偏离清单（评审通过后一次性收敛）

| # | 位置 | 问题 | 处置 |
|---|---|---|---|
| 1 | `internal/tenantconf` | `Store` 顶替 service，handler 直连数据层 | 补 `Service`，保持 internal（D-A：不迁 pkg） |
| 2 | `internal/tenantaccount/handler.go` | handler 直接查 `users` | 抽 `Service`（登录 / API Key 编排） |
| 3 | `internal/platformauth/handler.go` | handler 直接查 `platform_users` | 抽 `Service` |
| 4 | `internal/mcpgateway/provider.go` | `Provider` 兼做数据访问 | 数据访问下沉，`Provider` 只做协议转换 |
| 5 | `internal/apikey` + `internal/tenantaccount` | 能力与 HTTP 入口分居两包 | 按 §1.3（P2）已合规：能力留 `internal/apikey`，入口留 `internal/tenantaccount`，两层同在 internal（迁移已完成） |
| 6 | 全局 | 无 `module.go`，readme 3.4 未落地 | 按 §4 决策统一 |
| 7 | `readme.md` | 3.1 / 3.4 与本规范不一致 | 同步措辞 |
| 8 | `internal/testutil/isolation_test.go`、`authz_test.go` | 用例放在夹具库，与 §9.1 冲突 | 迁 `internal/integration/` |
| 9 | `internal/quota/service_test.go` 等 | 已直接 import `internal/testutil` | 随 §9.2 一并评估：要么下沉纯单测，要么迁 `internal/integration/` |
| 10 | 多个包（`roleRow`、`userRow`、`keyRow`、`toolRow`…） | 表模型命名不一，带 `Row` 后缀 | 统一改领域名词：`Role`、`User`、`APIKey`、`Tool` |
| 11 | 多处（`createRequest`、`statusRequest`、`setRequest`、`loginRequest`） | dto 散在 handler，命名无规则 | 统一为 service 包 `XxxInput` / `XxxOutput` |

---

## 12. 决策记录

| 编号 | 议题 | 结论 | 日期 |
|---|---|---|---|
| D-A | `tenantconf` 是否迁 `pkg` | **不迁**，保持 `internal`（对外暴露配置接口，属业务能力） | 2026-11-05 |
| D-B | `pkg` 是否允许带 HTTP handler | **禁止（P2）**：`pkg` 只放机制，能力与入口都在 `internal` | 2026-11-05 |
| D-C | fx：模块 `Module()` 还是集中装配 | **方案 A（模块自治）**：每个 internal 模块导出 `Module()` | 2026-11-05 |
| D-D | `internal` / `pkg` 划分标准 | **业务=对外暴露能力的模块 → internal；基础设施=只被业务调用的机制 → pkg**（已按此执行包迁移） | 2026-11-05 |
> 流程：本文件先改、评审通过后再改代码。代码与规范不一致时以本文件为准，不接受"先写再补规范"。
