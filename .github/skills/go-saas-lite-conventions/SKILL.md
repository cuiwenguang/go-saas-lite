---
name: go-saas-lite-conventions
description: "Use when writing, modifying, or reviewing Go code in the go-saas-lite project: deciding package layout (pkg vs internal), module file structure, naming (constructors, models, DTOs), database/table naming, fx assembly, layering, or test placement. Enforces the project's coding conventions and the pkg/internal boundary."
---

# go-saas-lite 编码规范

本 Skill 回答「代码怎么写、放哪里、叫什么」。行为与产品边界以 `readme.md` 为准，组织与命名以本文件为准。
写代码前先对照本文件；冲突时以本文件为准，不接受「先写再补规范」。

## 1. 顶层目录结构

```
go-saas-lite/
├── cmd/                          # 进程入口：只做启动与信号，不写业务
│   ├── api/main.go
│   ├── worker/main.go
│   └── migrate/main.go
├── internal/                     # 业务：按「平台侧 / 租户侧 / 共享能力」分组
│   ├── app/                      # 装配层：配置 + 双连接池 + 路由链 + fx 编排
│   ├── routes/                   # 路由链分组类型（Routes）：app 构造、各模块消费
│   ├── security/                 # 共享：认证与授权
│   │   ├── auth/                 # JWT 签发 / 解析 / Guard 中间件
│   │   ├── authz/                # 权限点、角色求值、缓存 + 失效广播
│   │   ├── apikey/               # 用户级 API Key（能力，无 HTTP 入口）
│   │   └── oauth/                # OAuth 2.1 服务 + HTTP 适配（单包）
│   ├── mcp/                      # MCP 协议 + 工具网关（协议/会话/JSON-RPC/上游代理/调用日志）
│   ├── quota/                    # 配额校验与限流
│   ├── worker/                   # 后台任务（维护锁 + 用量落库 + 归档清理）
│   ├── platform/                 # 平台控制台侧（package platform 聚合子模块）
│   │   ├── module.go             #   聚合 account + tenant
│   │   ├── account/              #   平台登录 + owner 写守卫
│   │   └── tenant/              #   租户 CRUD + 开通流水线 + 内置角色同步
│   ├── tenant/                   # 租户侧（package tenant 聚合子模块）
│   │   ├── module.go             #   聚合 account + conf
│   │   ├── account/              #   租户用户登录 + API Key 自助
│   │   └── conf/                 #   租户配置
│   ├── integration/              # 集成测试用例（真实 PG / Redis）
│   └── testutil/                 # 测试夹具库（只放夹具，不放用例）
├── pkg/                          # 基础设施：只被业务引用，不 import internal
│   ├── apperr/                   # 错误码分段 + 错误类型
│   ├── response/                 # code/msg/data 封装 + RequestID 中间件
│   ├── obs/                      # slog / otel / metrics / health / recover
│   ├── tenancy/                  # Scope / DB 门面 / Tx / GORM Callback
│   ├── redisx/                   # Redis 客户端 / 锁
│   └── migrate/                  # 迁移执行器
├── configs/
├── migrations/
│   ├── embed.go / tables.go      # 迁移嵌入 + 表归属白名单
│   ├── platform/                 # 平台组迁移
│   └── tenant/                   # 租户组迁移
└── go.mod
```

## 2. `pkg` 与 `internal` 的归属（一条标准）

**业务 = 对外暴露能力的模块**（HTTP 接口、MCP 端点）→ `internal`。
**基础设施 = 只被业务调用的机制**（数据库、缓存、配置、观测、迁移）→ `pkg`。

- 能力（做什么）：有 HTTP / MCP 入口、有产品业务语义 → `internal`。
- 机制（怎么做）：被能力调用、可整体拷走到另一个项目不改代码就编译 → `pkg`。
- **硬约束**：`pkg/*` 不得 import `internal/*`（CI 用 depguard 卡住）。
- **`pkg` 禁止带 HTTP handler**：所有 HTTP / MCP 适配一律放 `internal`。

## 3. 模块内结构（标准文件集）

一个业务模块（`internal/<domain>/` 或其子包）按职责命名，不按层建目录：

| 文件 | 职责 | 可否省略 |
|---|---|---|
| `handler.go` | 绑定、校验、统一响应；唯一知道 HTTP 的层 | 不可 |
| `service.go` | 业务规则、事务边界、跨组件编排；模块唯一对外入口 | 不可 |
| `model.go` | 表映射（领域名词，无 `Row`/`Model` 后缀） | 无表可省 |
| `dto.go` | service 入参 / 出参（`XxxInput` / `XxxOutput`） | 可选 |
| `perm.go` | 本模块权限点常量与登记 | 无权限点可省 |
| `module.go` | fx 装配（§6） | 必须 |

> service 过复杂时，按**业务场景语义**拆出辅助文件 / 结构降低复杂度（例：开通流水线 `provisioner.go`、角色同步 `sync.go`）。
> 这些辅助结构**没有固定命名**，按语义取名；一律包内私有，**不得被 handler 直接调用**；数据访问仍经 `tenancy.DB` / `PlatformDB`。

### 分层硬规则

```
handler ──► service ──► tenancy.DB / PlatformDB / Redis
```

- **MUST**：handler 只依赖 service、`response`、`apperr`、`authz.Authorizer`、绑定与校验。
- **MUST NOT**：handler 出现 `tenancy.DB`、`tenancy.PlatformDB`、`redis.Client`、`*gorm.DB`，也不写数据访问代码（哪怕一行）。
- **MUST**：事务只在 service 开启（`tenancy.DB.Tx`），handler 与 service 拆出的辅助结构不得 `Begin`/`Commit`。
- **MUST**：纯 CRUD 也保留 service，允许它只有转发。
- **MUST**：入参 / 出参定义在 service 包 `dto.go`，handler 直接绑定 `XxxInput`，不另建 `XxxRequest`。

## 4. 命名规范

### 4.1 Go 侧

| 对象 | 规则 | 示例 |
|---|---|---|
| 构造函数 | `NewXxx`，返回具体类型或接口；基础工具包单一入口用 `New` | `tenant.NewService`、`redisx.New` |
| 模块入口类型 | `Service` | `tenant.Service`、`apikey.Service` |
| 表模型（导出） | 领域名词，无 `Row`/`Model` 后缀 | `Role`、`APIKey`、`OAuthClient`、`PlatformUser` |
| 表投影（包内私有） | `<领域名>Record`，只声明本包用到的列，非权威模型 | `roleRecord`、`tenantRecord` |
| DTO 入参 / 出参 | `XxxInput` / `XxxOutput`，定义在 service 包 | `CreateTenantInput`、`LoginOutput` |
| HTTP 适配 | `Handler`，方法 `Register(group)` / `RegisterPublic(group)` | `tenant.Handler` |
| 中间件 | 返回 `gin.HandlerFunc` | `auth.Guard`、`quota.Middleware` |
| 选项 | `Option` + `WithXxx` | `auth.WithAccessTTL` |
| 接口 | 定义在使用方，动词或 `-er` 结尾 | `mcp.Provider`、`tenancy.Tx` |
| 表模型方法 | `TableName() string` 固定表名 | `func (Role) TableName() string` |

### 4.2 权威模型命名对照（当前模块集）

| 包 | 表 | 模型名 |
|---|---|---|
| `platform/tenant` | tenants / roles / users / user_roles | `Tenant` / `Role` / `User` / `UserRole` |
| `tenant/account` | users（投影） | `User` |
| `platform/account` | platform_users | `PlatformUser` |
| `security/apikey` | api_keys | `APIKey` |
| `security/oauth` | oauth_clients / oauth_grants | `OAuthClient` / `OAuthGrant` |
| `mcp` | mcp_tools / mcp_servers / mcp_call_log | `ToolRecord` / `Server` / `CallLogRecord` |
| `quota` | plans / tenants（投影） | `Plan` / `TenantPlan` |
| `tenant/conf` | tenant_configs | `Config` |

> 同一张共享框架表（users / roles / user_roles / tenant_configs）允许各包保留私有投影，
> 每包只声明自己用到的列；投影一律私有 `<领域名>Record`，权威模型只在 `model.go`。

### 4.3 数据库与配置命名

| 对象 | 规则 | 示例 |
|---|---|---|
| 表名 | 复数下划线；租户表必带 `tenant_id` | `tenant_configs`、`mcp_call_log` |
| 列名 | 下划线 | `provision_status` |
| 迁移文件 | `NNNNN_描述.sql`，组内递增 | `00006_oauth_grants.sql` |
| 配置字段 | `koanf` tag + yaml 项 + 默认值 + 校验 | `koanf:"platform_dsn"` |
| 权限点 | `obj:act`，obj 可点分层，动作禁止 `_` | `order:read`、`tenantconf:update` |
| 错误码 | 5 位分段；框架占 1–4 段，业务 9 段起 | `20005` |
| Redis key | `tenant:{tid}:biz:...`，由框架拼接，业务不自拼 | `tenant:{tid}:quota:qps:{sec}` |

## 5. 权限点与授权

- 权限点 = 代码常量 `authz.Perm{Obj, Act}`，落库格式 `obj:act`，通配 `*:*`。
- 标准动作固定 4 个 `read` / `create` / `update` / `delete`（`read` 覆盖列表与详情）。
- 特殊动作在模块 `perm.go` 用 `authz.Custom(obj, act)` 定义，并 `authz.Register` 登记。
- 内置角色权限以代码为准，`cmd/migrate -sync-roles` 每版同步到 DB。
- 业务只依赖 `authz.Authorizer{Enforce, HasRole}`，不直接读角色表。
- MCP 不建独立权限点：`mcp_tools.required_perms` 引用业务权限点，`tools/list` 与 `tools/call` 同源求值。

## 6. fx 装配（模块自治 + 路由自治）

每个 `internal` 业务模块导出 `Module() fx.Option`，**自包含**：`fx.Provide`（构造函数）+ `fx.Invoke`（挂自己的路由）。

```go
// internal/platform/tenant/module.go
func Module() fx.Option {
    return fx.Module("platform.tenant",
        fx.Provide(NewService, NewHandler),
        fx.Invoke(registerRoutes),
    )
}

func registerRoutes(routes routes.Routes, h *Handler) {
    h.Register(routes.Platform)
}
```

- 路由链分组类型 `Routes` 放在中立包 `internal/routes`，由 `internal/app` 构造并 Provide；模块只依赖 `routes.Routes`，不 import `internal/app`（避免循环）。
- `internal/app` 只做编排，不做各模块的路由挂载：`fx.Options(Core(cfg), tenant.Module(), tenantconf.Module(), ...)`。
- `fx.Invoke` 只做注册与触发（路由、定时任务、启动 server），不写业务逻辑。
- `*gorm.DB` 不出 `internal/app`（用包内类型 `tenantGorm` / `platformGorm` 包住）。
- OnStop 逆序执行：先停 HTTP，再关依赖；迁移 checksum 校验必须在 HTTP OnStart 之前。
- 依赖方向：`cmd → internal/app → internal/*`；`pkg/*` 不得 import `internal/*`。
- CI 必须跑 `fx.ValidateApp`（依赖图错误是运行时 panic，不是编译错误）。

## 7. 数据访问

- 业务代码只用 `tenancy.DB` / `tenancy.PlatformDB`，不出现 `*gorm.DB`。
- 平台组表只能经 `PlatformDB`；新增平台表必须同步 `pkg/tenancy` 的 `platformTables` 白名单。
- 原生 SQL 用 `Tx.Raw(sql, args...)`，只写单条参数化 SELECT，`tenant_id` 取自 Scope，且配一条双租户隔离用例。
- 跨租户维护操作（如清理）用 `Tx.DeleteAll`，不绕开 `tenancy.DB` 手写 SQL。

## 8. 错误、响应、日志

- 领域错误一律 `apperr.New` / `apperr.Wrap`；handler 只调 `response.Fail`，不手写状态码。
- 业务代码禁止 `c.JSON(500, ...)`。
- 日志统一 `slog.InfoContext(ctx, ...)`；`tenant_id` / `user_id` / `request_id` / `trace_id` 由 `pkg/obs` 自动补全。
- 禁止在 `pkg` 里 `os.Exit`、直接读环境变量（配置由 `cmd` 注入）。

## 9. 配置（新增配置项五件套，缺一不可）

1. struct 字段 + `koanf` tag；
2. `configs/config.yaml` 对应项（注释写明环境变量名）；
3. `DefaultConfig()` 默认值；
4. `validate()` 校验；
5. 至少一条 `LoadConfig` 绑定测试。

## 10. 迁移与表归属

- 两组目录：`migrations/platform`、`migrations/tenant`；版本表 `schema_migrations_platform` / `schema_migrations_tenant`。
- checksum 记在 `schema_migrations_checksum`；启动 `Verify` 失败即拒绝启动。
- 多进程并发迁移用 `pg_advisory_lock` 串行化。
- 新增表必须同时完成三件事：写迁移、登记归属白名单（平台组）、补双租户隔离用例（租户组）。

## 11. 测试

| 类型 | 放哪里 | 示例 |
|---|---|---|
| 单元测试（纯内存 / 桩） | 与被测代码同目录，`package xxx_test` | `internal/authz/authorizer_test.go` |
| 需要真实 PG / Redis 的集成测试 | `internal/integration/` | 隔离用例、OAuth 流程 |
| 装配 / 契约测试（路由、fx 图、生命周期） | `internal/app/` | `app_test.go`、`mcp_flow_test.go` |
| 夹具与断言库 | `internal/testutil/`，只放夹具，不放用例 | `Harness`、`SeedRole`、`MustNotSeeWidget` |

- 隔离用例不 mock `tenancy.DB`。
- 新增租户组表必须补「B 租户读 / 改 / 删不到 A」用例；新增 `Raw` SQL 必须补一条双租户用例。
- 数据库来源：`TEST_DATABASE_DSN` 优先；未设置时自动起 embedded-postgres；都不可用才 skip。
- 断言针对「自己的租户 / 记录」，不依赖全局计数。
- CI 必须跑：`gofmt -l`、`go vet`、`go test ./...`、`fx.ValidateApp`。
