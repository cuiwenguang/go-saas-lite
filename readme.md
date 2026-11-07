# go-saas-lite

## 这是什么

一个面向 AI / MCP 服务的 SaaS 通用底座：与业务无关，只干三件事——**规范的项目结构、租户数据隔离、租户管理**。MCP 服务建在同一套租户隔离之上。

底座包含这些能力：

| 能力 | 位置 |
| --- | --- |
| 租户生命周期（开通 / 注销 / 状态 / 配置） | 1.1 |
| 身份与鉴权（两套 actor + JWT + 角色权限） | 1.2 |
| 客户端接入（Web 应用 / Agent OAuth 2.1） | 1.2.5 |
| 基础数据表与表分组 | 1.3 |
| 统一基础组件 | 1.4 |
| 套餐与配额校验 | 1.5 |
| 文件存储接口 | 1.6 |
| 数据迁移与版本管理 | 1.7 |
| 异步任务接口 | 1.8 |
| 审计日志 | 1.9 |
| 隔离测试工具 | 1.10 |
| MCP 服务（传输 + 会话 + 工具注册表） | 1.11 |

六条设计原则：

1. **隔离方案可插拔**：默认「共享库 + 共享表 + `tenant_id`」，可扩展「共享库 + 独立 Schema」。
2. **依赖尽量少**：能用标准库就不引第三方。
3. **用 `go.uber.org/fx` 组装**：只当组装入口和生命周期管理。业务包只导出普通 `NewXxx(...)`，不用全局单例，不在 `init()` 里注册。
4. **默认两层半**：`handler → service → tenancy.DB`，有真实需求才加 `repo`。
5. **框架只给抽象、中间件、工具、规范**，业务逻辑不进框架。
6. **不绑定数据库**：隔离靠「`Scope` 注入 ctx + GORM Callback 自动注入 + 隔离测试」三重保障；机制只用到 `WHERE tenant_id = ?` 这类通用 SQL，不依赖数据库特有的隔离能力。

---

## 零、技术决策速查

开工前把这张表定死，后面不再改。

| # | 决策项 | 结论 |
| --- | --- | --- |
| D1 | 全局 ID | **UUIDv7**（PG 用 `uuid`，MySQL 用 `varchar(36)`） |
| D2 | ORM | **GORM v2**，只用 ORM + Callback |
| D3 | 数据库迁移 | **goose**（Schema 模式可用 atlas）；生产禁止 `AutoMigrate` |
| D4 | 用户与租户 | **`users.tenant_id` 直连**，一人一租户，`unique(tenant_id, email)` |
| D5 | 租户状态机 | `pending → active → suspended ⇄ active → expired → archived` |
| D6 | 错误码分段 | `1xxxx` 租户 / `2xxxx` 认证鉴权 / `3xxxx` 配额 / `4xxxx` 存储 / `9xxxx` 业务；框架只占 1~4 段 |
| D7 | API 路径 | 平台 `/api/platform/v1/*`，租户 `/api/tenant/v1/*`，两条中间件链分开注册；平台 token 不能访问租户接口，反之亦然；MCP 端点固定为 `/api/tenant/v1/mcp` |
| D8 | 基础设施与业务边界 | `pkg/` 是**基础设施**（DB / 缓存 / 配置 / 观测 / 迁移等机制），只被业务引用；`internal/` 是**业务**（对外暴露 HTTP / MCP 能力的模块）；用 lint 卡住 `pkg/*` 不 import `internal/*` |
| D9 | 模块划分 | 顶层按**模块（垂直切片）**切，模块内部再分层 |
| D10 | 配置分层 | 进程配置（yaml + env，重启生效）→ 平台运行时配置（DB）→ 租户配置（DB + 缓存），三层各用各的加载器 |
| D11 | 分层 | **两层半**：handler → service → `tenancy.DB`；`repo` 按需加；service 不持 `*gorm.DB` |
| D12 | 依赖注入 | **`go.uber.org/fx`**，每个业务模块导出 `Module() fx.Option`；`*gorm.DB` 只在 db 模块内可见；CI 必须跑 `fx.ValidateApp` |
| D13 | 隔离实现 | **中间件写入 `Scope` + GORM Callback 自动注入**；`tenancy.DB` 只从 ctx 取 `Scope`；平台侧用独立连接池；不设 `Scope.Skip` 开关 |
| D14 | 授权 | **自建 RBAC**：`roles` + `user_roles` + 代码里的权限点常量 |
| D15 | 库的划分 | **v1 单库**，逻辑上分「平台组 / 租户组」两组表；分库只改配置 |
| D16 | MCP 传输 | **Streamable HTTP 单端点**（`POST /api/tenant/v1/mcp`）；协议版本用 `MCP-Protocol-Version` 头协商；服务端只提供 HTTP 传输 |
| D17 | Agent 鉴权 | **OAuth 2.1（授权码 + PKCE，scope = `mcp`）** 或 **用户级 API Key（`sk_live_...`）**；两种方式都落到具体用户 |
| D18 | MCP 响应 | **JSON-RPC 2.0**，不走 `{code, msg, data}` 统一响应；平台错误码放在 `error.data.code` |
| D19 | 客户端类型 | **`web`**（租户自己的 Web 应用：JWT + refresh，CORS 白名单）与 **`agent`**（豆包 / WorkBuddy 等：OAuth 2.1，走 MCP） |
| D20 | 授权一致 | MCP **不引入独立权限点**：每个工具绑定平台业务权限点（`mcp_tools.required_perms`），求值走同一套 `internal/security/authz`；用户通过 Agent 能做的事 = 登录 SaaS 能做的事 |
| D21 | 权限动作 | 标准动作固定 4 个：`read` / `create` / `update` / `delete`（`read` 同时覆盖列表与详情）；特殊场景在模块 `perm.go` 用 `authz.Custom(obj, act)` 定义 |

---

## 一、基础能力

### 1. 租户生命周期

#### 1.1 租户表 `tenants`

| 字段 | 说明 |
| --- | --- |
| `id` | UUIDv7 |
| `name` / `code` | 名称 / 唯一短标识 |
| `domain` / `subdomain` | 租户访问域名 |
| `status` | `pending` / `active` / `suspended` / `expired` / `archived` |
| `plan_code` / `expire_at` | 套餐编码 / 到期时间；续费就是改 `expire_at` |
| `timezone` / `locale` | **必填**，日志、定时任务、报表都依赖它 |
| `contact_name` / `contact_phone` / `contact_email` | 联系人 |
| `provision_status` / `provision_step` | 开通进度，见 1.3 |
| `deleted_at` / `created_at` / `updated_at` | 软删时间与时间戳 |

状态决定中间件怎么拦：

| status | 行为 |
| --- | --- |
| `pending` | 403 `TENANT_NOT_READY`，只放行开通相关接口 |
| `active` | 放行 |
| `suspended` | 403 `TENANT_DISABLED` |
| `expired` | 403 `TENANT_EXPIRED` |
| `archived` | 404，对租户侧不可见 |

平台接口（`/api/platform/v1`）：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/tenants` | 列表：`page` / `size` / `status` / `keyword` / `plan_code` / `expire_before` |
| GET | `/tenants/summary` | 各状态计数（列表分段筛选用） |
| POST | `/tenants` | 开通，返回一次性初始密码 |
| GET | `/tenants/{id}` | 详情（含 `provision_log` 时间线） |
| PATCH | `/tenants/{id}` | 改名 / 换套餐 / 续费（改 `expire_at`；`expired` 续到未来自动回 `active`） |
| PATCH | `/tenants/{id}/status` | 状态机流转 |
| POST | `/tenants/{id}/retry` | 断点重跑开通流水线，重置管理员密码（一次性返回） |
| POST | `/tenants/{id}/admin-password` | 重置租户管理员密码 |
| GET | `/tenants/{id}/users` | 租户内用户与角色（只读） |
| GET | `/tenants/{id}/usage` | 套餐限额 + 近 30 天用量 + 近 7 天趋势 |
| DELETE | `/tenants/{id}` | 归档（转 `archived`） |
| GET / POST / PATCH | `/plans` | 套餐：列表（带引用租户数）/ 新建 / 改配额与启停（`code` 不可改） |
| GET | `/audit-logs` | 审计查询：分类 / 动作 / 级别 / 操作人 / 对象 / 时间范围 |
| GET | `/stats/overview`、`/stats/usage-top` | 总览计数 + 近 7 天趋势；用量排行 |

#### 1.2 租户配置 `tenant_configs`

- 按 `(tenant_id, namespace, key)` 存租户级配置：功能开关、存储配额、最大用户数等。
- `tenant_configs` 属租户组，只能经 `tenancy.DB` 访问；平台侧不得绕过租户 `Scope` 直接修改租户配置。
- 写入时发 redis pub/sub 通知其他实例；本地缓存带 TTL 和版本号，未命中回查 DB。
- 业务统一用 `tenantconf.Get[T](ctx, service, namespace, key)` 读取，不直接查表。
- 接口：`GET /api/tenant/v1/configs?namespace=`、`GET/PUT/DELETE /api/tenant/v1/configs/{namespace}/{key}`；读需 `tenantconf:read`，写需 `tenantconf:update`。

#### 1.3 租户开通流水线

创建租户不是插入一行数据，而是一条**幂等、可重试、进度落库**的流水线：

```
写 tenants(pending)
→ 初始化 tenant_configs
→ 建内置角色 roles（admin / member，权限取自代码常量）
→ 建租户管理员 users（1 条，bcrypt，status = active）
→ 绑定 user_roles（该用户 ← admin）
→ [Schema 模式] 建 schema + 执行租户迁移
→ 灌种子数据
→ status = active
```

实现约定：

- `tenants` 属平台组、其余表属租户组，两个连接池**不能共享事务**；流水线按「平台段事务 + 租户段事务」分步提交，不宣称跨组原子。
- 平台段：生成租户 ID → 在 `tenancy.PlatformDB` 事务内写 `tenants(pending)` 与初始 `provision_status` → 提交，使租户可被查到。
- 租户段：用 `Scope{TenantID: 新租户 ID}` 在 `tenancy.DB.Tx(ctx, fn)` 内建内置角色、管理员用户、`user_roles` 绑定；该段单次提交。
- Schema 模式的建 schema 与租户迁移**不在事务内**，作为独立步骤执行，每步结束单独落库。
- 每步按 `unique(tenant_id, code)` / `unique(tenant_id, user_id, role_id)` 幂等，重跑不产生脏数据。
- `provision_step` 必须在**该步骤提交之后**单独写入（独立提交），否则事务回滚会把进度一起回滚、跳过逻辑失效。
- 任一必需步骤失败 → `provision_status = failed`，租户保持 `pending` 等重试；重试从 `provision_step` 之后继续。
- 可以在 worker 里异步执行，接口先返回 `pending`。

```go
// internal/platform/tenants/provisioner.go —— 当前实现：Service.provision 在租户事务内
// 依次 ensureRole / ensureUser / bindRole，每步先查后建，重跑幂等；reset 为真时覆写管理员密码。
// 步骤进度与历史由 tenants.provision_step + provision_log(jsonb) 记录：
// [{step, ok, error, at}]；失败后 retry 从断点继续，管理员密码重置为新的一次性密码。
```

#### 1.4 租户注销

三步，**不允许一步硬删**：

1. 注销 → status 改为 `archived`，立即拒绝登录（冷却期内可恢复）。
2. 冷却 N 天（可配置），到期后由 `cmd/worker` 清理框架表（users / roles / user_roles / tenant_configs）并写 `deleted_at`。
3. 业务表与文件对象的清理由**业务实现**，框架只提供钩子与调度。

#### 1.5 登录与租户上下文

- **登录是全局的**：租户用户登录只输邮箱和密码，不需要填租户、也不需要先知道子域名。邮箱全局唯一，登录时先查平台组 `user_identities` 定位到 `(tenant_id, user_id)`，再用 `tenancy.DB` 在**该租户作用域内**校验密码与账号状态。任何人不能跨租户查 `users`。
- **`user_identities` 只是登录索引**：只存 `identifier_type` / `identifier` / `tenant_id` / `user_id`，不存密码与资料。租户库 `users` 是唯一真相源，索引是它的派生投影。
- **写入与修复**：开通租户（管理员）、创建成员时在租户库写入成功后登记索引；归档租户时清理。索引与租户库不同连接池，不做双向同步，用「幂等写 + 对账修复」保证最终一致（`cmd/migrate -command repair-identities`，建议每日跑一次）。
- **唯一性口径**：v1 一人一租户，`(identifier_type, identifier)` 全局唯一；创建成员前先查索引，命中就返回「邮箱已被使用」。
- **请求头的语义**：登录后每个请求带 `X-Tenant-Id`（前端从 `/me` 得到），但**服务端以令牌里的 `tenant_id` 为准**；头只作为「当前选择」的显式声明，为后续一人多租户切换留扩展位，越权不靠头。
- 解析结果放进 `Scope` 注入 context；下游一律用 `tenancy.ScopeFrom(ctx)` 读取，不再额外传参。
- 租户不存在 / 已禁用 / 已过期 → 登录时直接拒绝，并返回具体原因（`tenant_expired` / `tenant_disabled`）；令牌链路再次复查，租户被停用或到期后旧令牌立即失效。到期采用**读时判定**：`status = active` 且 `expire_at` 已过即按 `expired` 处理，**不写库**（`internal/app/tenantgate.go`）。
- **子域名只作可选接入**：`tenancy.base_domain` + `TenantResolver` 保留给自定义域名 / 品牌 / SSO 场景，不作为默认登录路径（OAuth 注册与授权仍需租户上下文，走该解析）。
- 数据库状态由 worker 每日流转（`ExpireTenants`：`active` 且已过期 → `expired`），请求侧判定与库内状态互补。
- **异步任务**：用 `context.WithoutCancel(ctx)` 派生并显式重建 `Scope`（项目当前没有 `tenantctx` 包）；任务 payload 必须显式带 `tenant_id`，worker 侧重建 `Scope`。只复制 ctx 在队列场景一定会丢。

#### 1.6 数据隔离层

- **唯一入口 `tenancy.DB`**：`Tx(ctx, fn)` / `Read(ctx, fn)`，内部负责开事务、绑定 `Scope`、把 `tenant_id` 交给 Callback。
- 共享表模式：GORM `Create` / `Find` / `Update` / `Delete` 在绑定 `Scope` 后由 Callback 自动填充或补租户条件；批量操作复用同一套 GORM 操作路径。
- 原生 SQL 是例外路径：只用于 ORM 表达不了的分组、聚合、报表；必须参数化、必须自带租户条件、必须配双租户隔离用例，写法见 3.2.1。
- 平台侧用独立的 `tenancy.PlatformDB`（独立连接池，不注入 `tenant_id`），不用开关切换。
- Schema 模式只定义接口 `TenantSchemaManager`（Create / Drop / Migrate），实现放扩展包。
- 建表规范从第一张表开始执行：`tenant_id NOT NULL` + 独立索引 + `(tenant_id, id)` 唯一约束。

### 2. 身份与鉴权（两套体系物理隔离）

#### 2.1 两套身份
- **平台管理员**：存放在平台库 `platform_users`，不属于任何租户，全局权限
- **租户用户**：存放在**租户隔离表 `users`**，直接带 `tenant_id`（v1 一人一租户）
- **全局登录索引**：平台组 `user_identities` 记录「登录标识 → (tenant_id, user_id)」，让登录不需要先知道租户。索引不含密码与资料，由开通 / 建成员写入、归档清理、对账修复（见 1.5）

#### 2.2 Token 设计（golang-jwt v5）
- Claim 必备：`sub`(user_id)、`actor_type`(`platform|tenant`)、`tenant_id`（租户 token 才有）、`client_id`（Agent token 才有）、`scope`、`typ`（`access` / `refresh`）、`jti`、`aud`、`exp`；`email` 仅用于审计展示
- **`aud` 区分入口**：Web 应用 `aud = tenant-api`，Agent 客户端 `aud = mcp`，平台控制台 `aud = platform-api`；跨入口使用一律拒绝
- **`aud` + `actor_type` 双校验**：平台路由链拒绝 `tenant` token，租户路由链拒绝 `platform` token
- 刷新：refresh token **轮换**（rotation）。平台控制台已实现：refresh 令牌走 httpOnly cookie `saas_platform_refresh`（`path=/api/platform/v1/auth`），jti 存 `platform_users.refresh_jti` 且只保留最近一次（单会话，轮换即作废旧会话）；`typ=refresh` 的令牌被 Guard 拒绝，不能访问业务接口
- 登出：平台侧清空 `refresh_jti` 并清 cookie；租户 Web 侧同样已实现（cookie `saas_tenant_refresh`，`path=/api/tenant/v1/auth`，jti 存 `users.refresh_jti`，单会话轮换，登出 / 改密即失效；登录只需邮箱密码，用户无需关心租户）
- **不要把完整权限集合塞进 JWT**（权限变更无法即时生效）；JWT 只放身份，权限判定走 `internal/authz` 的进程内角色缓存（变更时 redis pub/sub 广播失效）

#### 2.3 权限模型：角色表 + 代码权限点

v1 只有内置角色，不开放自定义角色。授权统一通过 `internal/authz` 接口完成，业务代码只调 `Enforce` / `HasRole`，不直接碰表。

**权限点 = 资源 + 动作**（不建权限字典表）

标准动作就 4 个，默认全用它：

| 动作 | 覆盖范围 | 说明 |
| --- | --- | --- |
| `read` | 列表 + 详情 + 单条查询 | **不区分 list / detail** |
| `create` | 新增 | |
| `update` | 修改（含状态流转中的字段变更） | |
| `delete` | 删除 | 软删与硬删同为 `delete` |

模块有特殊场景时，在**该模块的 `perm.go`** 里定义专属动作，例如 `export`、`approve`、`publish`、`assign`、`purge`。

```go
// internal/security/authz/perm.go
type Perm struct{ Obj, Act string }

func Read(obj string) Perm        { return Perm{obj, "read"} }
func Create(obj string) Perm      { return Perm{obj, "create"} }
func Update(obj string) Perm      { return Perm{obj, "update"} }
func Delete(obj string) Perm      { return Perm{obj, "delete"} }
func Custom(obj, act string) Perm { return Perm{obj, act} }

func All() []Perm              // 启动校验 + 内置角色同步用
func (p Perm) String() string  // "order:read"（Obj 内禁止出现 ':'）

// 模块内定义
var (
    OrderRead   = Read("order")              // order:read
    OrderExport = Custom("order", "export")  // order:export
    InvoiceRead = Read("billing.invoice")    // billing.invoice:read
)
```

规矩：

- 落库格式固定 `{Obj}:{Act}`；`Obj` 用 `.` 分层（`billing.invoice`），**禁止含 `:`**；`Act` 用短动词，不写下划线组合（不写 `list_with_filter`）
- **不按接口 / 按钮 / 列表详情拆分**：一个资源最多 4 个标准动作，足够覆盖绝大多数页面
- 支持通配 `*:*` = 全部权限；`admin` 的 `permissions` 直接存 `["*:*"]`，新增权限点不用回填 admin
- 权限点全部来自代码常量；**MCP 工具不新增权限点**，`required_perms` 直接引用（如 `["order:read"]`）
- 自定义动作集中写在模块 `perm.go`，不允许在 handler 里临时拼字符串

**两张表，都在租户组（受 `tenant_id` 隔离，走 `tenancy.DB`）**

```sql
-- roles：角色定义
id          uuid      PK (UUIDv7)
tenant_id   uuid      NOT NULL            -- 独立索引
code        varchar   NOT NULL            -- admin / member / 自定义
name        varchar   NOT NULL
permissions jsonb     NOT NULL            -- ["user:read","user:create"]
is_builtin  boolean   NOT NULL DEFAULT false
created_at / updated_at
unique(tenant_id, code)
```

```sql
-- user_roles：角色绑定（一人可多角色）
tenant_id  uuid  NOT NULL
user_id    uuid  NOT NULL
role_id    uuid  NOT NULL
created_at
unique(tenant_id, user_id, role_id)
```

- 两张表用物理删：删除前校验无绑定 + 写审计日志（软删会与 `unique(tenant_id, code)` 冲突）
- `users` 表**不存角色**，角色一律经 `user_roles`
- 权限变更审计落 `tenant_operation_log`（谁、何时、把哪个角色的哪些权限改成什么）

**内置角色与权限同步**

| `code` | 说明 | `permissions` | 可删 | 可改权限 |
| --- | --- | --- | --- | --- |
| `admin` | 租户管理员，全部业务权限 | `["*:*"]` | ❌ | ❌ |
| `member` | 普通成员，最小权限集 | 代码常量 `MemberPerms` | ❌ | ❌ |

- **内置角色以代码为准，DB 只存一份结果**：`cmd/migrate` 加一个幂等步骤 `SyncBuiltinRoles`，每次发版把 `admin` / `member` 的 `permissions` 覆写成代码常量。
- 不要只在开通时写入一次，否则老租户拿不到新增权限点。
- **最后管理员保护**：删除用户 / 解绑角色前，事务内校验该租户仍有 ≥1 个 active 且持 `admin` 的用户，否则拒绝。

**求值与缓存**

- 进程内缓存：`tenant_id → {version, code → permSet}`，`jsonb` 解析后缓存，**不每次解析**
- 用户角色绑定：每次请求经 `Source` 读取 `roles` / `user_roles`，按租户缓存在进程内（`tenantState`），不是每次解析；变更后用 redis pub/sub 广播失效本地缓存。**未使用 redis set 缓存用户角色**。
- **角色权限 / 绑定变更后广播失效**：redis pub/sub `authz:reload:{tenant_id}`，多副本下否则会出现"权限时好时坏"
- 业务只依赖 `internal/authz`，不直接碰表：

```go
type Authorizer interface {
    Enforce(ctx context.Context, obj, act string) error // 无权限返回 2xxxx 错误
    HasRole(ctx context.Context, code string) bool
}
```

- `permission` 中间件：`tenant_id` 只从 `Scope` 取、`user_id` 只从 JWT/ctx 取，**绝不读请求体 / query / header 里的角色或权限字段**
- 平台侧不建角色表：`platform_users.role` 只有 `owner` / `viewer`，权限点在 `internal/security/authz/platform.go` 用静态矩阵映射（owner 全部、viewer 只读），路由级挂 `authz.PlatformRequire(perm)`，`GET /me` 下发 `permissions[]` 供前端控制菜单与按钮。

#### 2.4 密码与登录安全

- 密码哈希：`bcrypt`（cost 可配）
- 登录失败锁定：按 `tenant:{tid}:login:fail:{account}` 计数，超阈值锁定 N 分钟
- 最小密码策略：长度 + 复杂度，写入 `users` 前校验
- 开通时生成的初始密码标记 `must_change_password`，首次登录强制修改
- 密码重置 / 改密后递增用户 `token_version`，使旧 token 失效

#### 2.5 客户端类型与接入方式

两类客户端，共用一套身份与权限：

| 类型 | 使用者 | 入口 | 凭证 |
| --- | --- | --- | --- |
| `web` | 租户自己的 Web 应用（浏览器） | `/api/tenant/v1/*` | 登录拿 JWT：access 15 分钟 + refresh 7 天，轮换 |
| `agent` | 豆包 / WorkBuddy 等第三方 AI 客户端 | `/api/tenant/v1/mcp` | OAuth 2.1 授权码 + PKCE 换出的 Bearer |

**Web 应用**

- 登录：`POST /api/tenant/v1/auth/login`（email + password，**无需租户参数**；内部先查 `user_identities` 定位租户），返回 access token；初始管理员首次登录需改密。
- access token 走 `Authorization: Bearer`，由前端持有；refresh token 放 `httpOnly` cookie。
- CORS 只放行 `tenants.domain` / `subdomain` 命中的来源；`OPTIONS` 预检不进鉴权链。
- 401 → 前端用 refresh 换新 access；refresh 也失败则回登录页。
- Web 的 access token 只用于 `/api/tenant/v1/*` 业务接口，**不用于 MCP**（`aud = tenant-api`）。

**Agent 客户端**

第三方 AI 客户端按 MCP 的 OAuth 2.1 要求接入：

```
GET  /.well-known/oauth-protected-resource      # 资源元信息（RFC 9728）
GET  /.well-known/oauth-authorization-server    # 授权服务器元信息
POST /api/tenant/v1/oauth/register              # 动态客户端注册（RFC 7591）
GET  /api/tenant/v1/oauth/authorize             # 授权码 + PKCE，复用租户登录页
GET  /api/tenant/v1/oauth/consent               # 同意页：展示租户、用户、scope
POST /api/tenant/v1/oauth/token                 # 换 access / refresh token
POST /api/tenant/v1/oauth/revoke                # 撤销授权
```

- 授权码有效期 10 分钟、**单次使用**，存 redis；`code_challenge_method` 只接受 `S256`。
- 动态注册只允许 HTTPS `redirect_uri`；开发环境允许 `http://127.0.0.1` / `http://localhost`。授权和换令牌时必须精确匹配已注册的 `(client_id, redirect_uri)`，禁止通配符重定向。
- 授权请求必须携带 `state`，客户端用它防跨站请求伪造；OIDC `nonce` 不是本协议必需项，不采用时不发 `id_token`。
- `oauth_clients` 至少包含 `client_id`、`tenant_id`、`client_name`、`redirect_uris`（JSON）、`grant_types`、`token_endpoint_auth_method`、`created_at`、`revoked_at`。
- `oauth_grants` 至少包含 `tenant_id`、`user_id`、`client_id`、`scope`、`revoked_at`、`created_at`、`updated_at`；`(tenant_id, user_id, client_id)` 唯一。
- 授权码 Redis 结构至少包含 `tenant_id`、`user_id`、`client_id`、`redirect_uri`、`scope`、`code_challenge`、`expires_at`；一次成功校验后原子删除。
- `scope` 固定为 `mcp`；工具能不能用，由该用户在 SaaS 里的业务权限点决定，与登录后完全一致。
- access token 内容：`sub` = user_id、`client_id`、`tenant_id`、`scope`、`jti`、`exp`、`aud = mcp`；refresh token 只存 sha256 摘要及轮换关系，不存明文。
- refresh token 轮换后，旧摘要立即失效并记录关联的 token family；同一 family 重复使用旧 token 时撤销该 family 的所有 refresh token。
- 每次 MCP 调用按 `sub` 求值，Agent 拿到的能力等于该用户在 Web 里看到的能力。
- token 过期返回 `401` + `WWW-Authenticate: Bearer error="invalid_token"`，客户端重新走授权。
- 撤销授权：先在事务内置 `oauth_grants.revoked_at`，再删除该 grant 对应的 refresh token 摘要并作废绑定该 grant 的 `Mcp-Session-Id`；数据库状态和 session/Redis 清理失败不得反向恢复已撤销授权。

**API Key（用户级）**

不支持 OAuth 的客户端，用用户在 Web 端自助生成的 API Key 接入：

- 明文只在创建时返回一次；库里只存 `key_prefix`（如 `sk_live_ab12`，用于列表展示）与 `sha256(key)`。
- 请求头 `Authorization: Bearer sk_live_...`。
- 鉴权顺序：`sha256(key)` → 走平台连接池按 `key_hash` 查 `api_keys` → 得到 `(tenant_id, user_id)` → 组 `Scope` → 后续与 OAuth 同一条链路。
- 权限按该用户的角色求值，与 Web、OAuth 完全一致。
- 审计：`mcp_call_log` 的 `client_id` 记 `apikey:{id}`，能追到人和 Key。
- 撤销 / 轮换：`revoked_at` 置位 + 删缓存，同一请求内生效。
- 管理接口：`GET/POST/DELETE /api/tenant/v1/api-keys`；用户只能操作自己的 Key，`admin` 可看租户内全部；权限点 `apikey:read` / `apikey:create` / `apikey:revoke`。

**会话与审计**

- `Mcp-Session-Id` 绑定 `(tenant_id, user_id, client_id)`，三者任一不符返回 404。
- `mcp_call_log` 记录 `user_id` 与 `client_id`，能追到「哪个用户的哪个 Agent 调了什么工具」。

### 3. 基础数据表

#### 3.1 平台组表（不受租户隔离）

| 表 | 用途 |
| --- | --- |
| `tenants` | 租户主表 |
| `plans` | 套餐定义与配额 JSON |
| `usage_daily` | 每日用量快照（worker 从 Redis 落库） |
| `platform_users` | 平台管理员账号（含 `role` 单角色枚举） |
| `oauth_clients` | 动态注册的 Agent 客户端（`client_id`、回调地址、授权类型） |
| `api_keys` | 用户级 API Key（`key_hash` 全局唯一，鉴权前定位到 `tenant_id` + `user_id`） |
| `user_identities` | 全局登录索引：`unique(identifier_type, identifier)` → `(tenant_id, user_id)`，只存定位不存密码 |
| `system_operation_log` | 平台管理员操作日志（开通 / 禁用 / 删除租户等） |

> `oauth_clients`、`api_keys`、`user_identities` 要**在确定租户之前**查询，所以放平台组；`api_keys` 每行都带 `tenant_id` + `user_id`，不丢审计粒度。

#### 3.2 租户组表（**受 `tenant_id` 隔离**）

| 表 | 用途 | 关键约束 |
| --- | --- | --- |
| `tenant_configs` | 租户级 KV 配置 | `unique(tenant_id, namespace, key)` |
| `users` | 租户内用户账号（登录凭证 + 基本资料） | `unique(tenant_id, email)`、`unique(tenant_id, phone)`；邮箱另由平台组索引保证全局唯一 |
| `roles` | 角色定义（内置） | `unique(tenant_id, code)`，`permissions` 存 `jsonb` |
| `user_roles` | 用户 ↔ 角色绑定 | `unique(tenant_id, user_id, role_id)` |
| `oauth_grants` | 用户对 Agent 客户端的授权记录 | `unique(tenant_id, user_id, client_id)`；`revoked_at` 软标记 |
| `mcp_servers` | 租户配置的上游 MCP 服务（地址、凭证、启用状态） | `unique(tenant_id, name)`；凭证加密存储 |
| `mcp_tools` | 工具注册表（工具名、入参 schema、`required_perms`、启用状态） | `unique(tenant_id, server_id, name)` |
| `mcp_call_log` | 工具调用日志（谁、哪个 Agent 客户端、哪个工具、状态、耗时、用量） | 按 `tenant_id` 隔离，按月归档 |
| `tenant_operation_log` | 租户内操作日志（与平台操作日志分开） | 按 `tenant_id` 隔离 |

> 角色与权限绑定**建表**（`roles` / `user_roles`，见 1.2.3），两者都在**租户组**、受 `tenant_id` 隔离。

**建表规则（3.2 的每张表都适用）**

- `tenant_id NOT NULL` + 独立索引，从第一张表起就是硬约束
- 唯一索引一律带 `tenant_id` 前缀：`unique(tenant_id, email)`，**不是** `unique(email)`
- 不建跨租户外键；外键只能在同一个 `tenant_id` 内引用
- `users` 表**不存角色**，角色绑定在 `user_roles` 里
- 所有读写都必须经 `tenancy.DB`，`tenant_id` 由 Callback 填充，业务代码不传

#### 3.3 登录前置条件

`users` 在隔离区，所以租户用户登录时先通过平台组 `user_identities` 把邮箱换成 `tenant_id`，再在租户作用域内校验密码；不允许跨租户直接查 `users`。

如果业务要求「全局登录后再选租户 / 切租户」，`users` 就不能放在隔离区，改为平台库建 `accounts`（只存登录凭证 + 当前租户）。这条必须在接第一个登录接口前确认。

#### 3.4 平台组 / 租户组与分库

**v1 用同一个物理库，逻辑上分两组表。**

| 组 | 表 | 访问门面 | 租户 Callback |
| --- | --- | --- | --- |
| **平台组** | `tenants`、`plans`、`usage_daily`、`platform_users`、`oauth_clients`、`api_keys`、`system_operation_log` | `tenancy.PlatformDB` | ❌ 不注册 |
| **基础设施** | `schema_migrations` | `tenancy.PlatformDB` | ❌ 不注册 |
| **租户组** | `tenant_configs`、`users`、`roles`、`user_roles`、`oauth_grants`、`mcp_servers`、`mcp_tools`、`mcp_call_log`、`tenant_operation_log`、后续所有业务表 | `tenancy.DB` | ✅ 自动注入 |

**分库只是一个配置项**：

```yaml
database:
  tenant_dsn:   ${DB_DSN}   # 与 platform_dsn 相同 = 单库
  platform_dsn: ${DB_DSN}
```

`tenancy.DB` 与 `tenancy.PlatformDB` 是两个类型、两个连接池，分库时只改配置 + 跑两轮迁移，代码不动。

**单库时必须做的三件事**：

1. **迁移目录分离**：`migrations/platform/` 与 `migrations/tenant/`。
2. **表归属白名单**：`pkg/tenancy/gorm.go` 维护 `platformTables` 白名单（通过 `PlatformTables()` 导出拷贝，编译期常量，不是运行时配置）。
3. **双向一致性测试**：`migrations/platform/*.sql` 里的表名必须**全部**在白名单内；`migrations/tenant/*.sql` 的表名必须**一个都不**在白名单内。

> 表名前缀（如 `sys_`）只帮人读懂，不能替代白名单。

**拆成两个库的触发条件**：

- 一个平台库被多个产品共用
- 合规要求平台元数据与租户数据分开存储 / 加密 / 审计
- 平台库查询压力挤占租户业务，需要独立扩缩容或独立备份
- 走 Schema-per-tenant 模式

**拆库后的限制**：两套连接池与迁移、无法跨库事务（开通流水线要拆步）、无法跨库 join（需两次查询 + 应用层聚合）、本地开发多起一个库。

### 4. 统一基础组件

1. **统一响应**：`{code, msg, data, request_id}`；错误码见 D6，含 SaaS 特有码（租户禁用 / 过期、配额超限、跨租户越权）。**MCP 端点例外**，按 JSON-RPC 2.0 返回（见 1.11）。
2. **日志**：`slog` + `InfoContext`，`tenant_id / user_id / request_id / trace_id` 由 Handler 从 ctx 自动补全，业务代码不手写。
3. **可观测性**
   - OpenTelemetry trace，trace_id 贯穿日志
   - `/metrics`（Prometheus）：请求量 / 耗时 / 错误率 / 租户维度配额用量
   - `/health`（liveness）与 `/readyz`（DB、Redis 连通性）
   - pprof 挂**独立内网端口**，不暴露在业务端口
4. **全局 panic 恢复**：单租户请求 panic 不拖垮进程，同时上报 metrics 与日志。
5. **配置分三层**（见 D10），env 优先级高于 yaml。
6. **写接口幂等**：`Idempotency-Key` 工具（redis 存 key → 结果），用于创建租户、扣配额等。
7. **优雅关闭**：signal → 停止接新请求 → 等待在途请求 → 关闭连接池。

### 5. 套餐与配额

- 套餐 = 一组配额上限，**用 JSON 存在平台组的 `plans.quota`**：`{"max_users":10,"storage_gb":5,"api_qps":20,"mcp_calls":100000,"mcp_tokens":5000000,"mcp_sessions":50,"features":["sms"]}`。
- `plans` 至少包含 `id`、`code`（唯一）、`name`、`quota`（JSON）、`enabled`、`created_at`、`updated_at`。
- 套餐管理：`GET /api/platform/v1/plans`（带引用租户数）、`POST /plans`（新建）、`PATCH /plans/{id}`（改名 / 改配额 / 启停；`code` 不可改）。改动经 `quota.PlanStore` 的 30s 缓存，最迟 30 秒对租户生效。
- 套餐归属平台组，由 `PlatformDB` 读取；租户只通过 `tenants.plan_code` 引用套餐，不把套餐记录复制进租户组。
- 配额校验：`quota.Service.Check/Consume` 读租户套餐 + 当天用量，超限返回 `3xxxx`；计次入口调用 `Consume`
- 用量统计：Redis hash 按天计数，worker 定期落 `usage_daily`
- `plans.quota` 的字段：`max_users` / `storage_gb` / `api_qps` / `mcp_calls` / `mcp_tokens` / `mcp_sessions` / `features`；值为 0 表示不限
- MCP 三个指标：`mcp_calls`（调用次数，周期累计）、`mcp_tokens`（上游返回的 token 数）、`mcp_sessions`（并发会话上限）
- 续费 = 改 `tenants.expire_at` 一个字段

### 6. 文件存储

- 底座定义 `Storage` 接口：`Put / Get / Delete / Exists / SignedURL`（`pkg/storage`）
- 已实现三种驱动：`local`（默认，HMAC 签名下载）、`cos`（腾讯云）、`oss`（阿里云）；由 `storage.driver` 切换，云驱动缺凭据时启动直接报错
- key 规范 `tenant/{tenant_id}/{biz}/{uuid}`，**由框架拼接**，业务不得自拼
- 元数据落 `tenant_files`；上传受 `storage.max_upload_mb` 与套餐 `storage_gb` 双重限制
- 删除先删元数据，再投递 `file.delete` 异步删对象（投递失败回退同步删除）
- 归档租户清理时，文件清理器（worker `Cleaner`）删除对象与元数据

### 7. 数据迁移与版本管理

- 两类迁移严格分开：
  - **平台迁移** `migrations/platform/`：`tenants`、`platform_users` 等公共表
  - **租户迁移** `migrations/tenant/`：业务表（共享表模式只跑一次；Schema 模式每个 schema 各跑一遍）
- 迁移脚本以 PostgreSQL 编写（`uuid` / `jsonb`）；接入 MySQL 时新增一套等价脚本与同一套隔离用例，`pkg/tenancy` 不含方言分支。
- 单库时平台组与租户组各用一张版本表：`schema_migrations_platform` / `schema_migrations_tenant`，避免两组版本号互相污染。
- checksum 单独记在 `schema_migrations_checksum`（group + version + filename + checksum）；每次执行前比对，脚本改了却没重新迁移直接拒绝。
- 迁移必须**幂等、可重入**；`cmd/migrate -command up|status|verify -group platform|tenant|all` 覆盖执行、查看与校验；`-sync-roles` 同步内置角色权限。
- Schema 模式需提供"存量租户批量补迁移"执行器（`cmd/migrate --all-tenants`），单租户失败可跳过并记录；Schema 模式未启用时该参数直接报错，不静默忽略

### 8. 异步任务

- 已接入 asynq（`pkg/queue`），生产端与消费端共用 `internal/tasktype` 的类型与 payload
- 已注册任务：`sms.send`、`file.delete`、`tenant.purge`、`tenant.provision.retry`、`demo.heartbeat`
- `tenant.provision.retry` 走 `RetryProvisionExisting`：保留管理员现有密码续跑开通，不重新生成一次性密码
- 后台定时任务由 `internal/worker` 承担（用量落库、到期流转、归档清理、审计保留期清理）；多副本用 redis 锁 `worker:lock:maintenance` 串行
- `queue.enabled=false` 或未配置 Redis 时自动降级：API 只提供同步路径，worker 不启动队列
- **dev 快捷**：`app.env=dev` 时 `internal/app` 会把 `worker.New` 与 `worker.NewQueueServer` 一并挂到 api 进程，省去单独起服务；生产仍用独立 `cmd/worker` 进程

### 9. 审计日志

- 两个通道，**不要混**：平台操作日志（`system_operation_log`）、租户内操作日志（`tenant_operation_log`，按 `tenant_id` 隔离）
- 平台通道（`internal/platform/audit`）：`Record` 写入 + `GET /api/platform/v1/audit-logs` 查询，支持分类 / 动作 / 级别 / 操作人 / 对象 / 时间范围筛选
- 租户通道（`internal/tenant/audit`）：`Record` 写入 + `GET /api/tenant/v1/audit-logs` 查询（`audit:read`，仅 admin），支持分类 / 动作 / 级别 / 操作人 / 时间范围筛选
- 字段：`category` + `action`（代码常量）+ `level`（`info` / `warn`）+ `result` + 操作人（平台为 `actor_type` / `actor_id` / `actor_email`，租户为 `actor_id` / `actor_email`）+ `object_type` / `object_id` + `summary` + `before_data` / `after_data` + `request_id` + `ip`
- 平台已埋点：租户开通 / 修改 / 续费 / 变更状态（归档记 warn）/ 重试开通 / 重置密码（warn）、套餐新建 / 修改 / 启停、平台登录成功与失败（warn）/ 改密（warn）、**跨租户查看租户用户**（`security`，warn）
- 租户已埋点：成员创建 / 修改 / 重置密码 / 变更角色、API Key 创建 / 撤销、MCP 服务与工具配置、文件上传 / 删除、OAuth 授权撤销
- 操作人邮箱来自 JWT `email` claim（写入 `Scope.ActorEmail`），不额外查库；`request_id` / `ip` 由 `pkg/response` 中间件写入 ctx
- 保留期：`audit.retention_days`（进程配置，默认 90），worker 每次 tick 清理超期的**平台**审计记录
- **不允许导出**：日志只用于页面查询与评审，按分类与风险级别筛选

### 10. 隔离测试工具 `internal/testutil`

- 一键起 PostgreSQL（testcontainers 或 embedded-postgres；也支持 `TEST_DATABASE_DSN` 复用现有库）→ 建两个租户 → 提供断言工具；接入 MySQL 时复用同一套断言
- 必备用例：**A 租户读不到 / 改不到 / 删不到 B 租户数据**，覆盖每张业务表
- 必测：漏写 `tenant_id` 的跨租户读 / 写场景。
- 放在 `internal/`，不对外复用。

### 11. MCP 服务

平台同时扮演两个角色：**MCP Server**（暴露平台自有工具）和 **MCP Gateway**（代理租户配置的上游 MCP 服务）。两者共用一张工具注册表。

**传输**

- 协议：MCP **Streamable HTTP**，单端点 `POST /api/tenant/v1/mcp`。
- 请求 `Content-Type: application/json`；响应按需返回 `application/json` 或 `text/event-stream`。
- 协议版本用 `MCP-Protocol-Version` 头协商，`initialize` 时回传服务端支持的版本。
- 服务端只跑 HTTP 传输，不跑 stdio。

**鉴权**

- `Authorization: Bearer <token>`，**只有两种来源**：Agent 客户端 OAuth 2.1 换出的 token（`aud = mcp`）与用户级 API Key（`sk_live_...`），见 1.2.5。
- Web 应用的租户 JWT（`aud = tenant-api`）**不能访问 MCP**，直接返回 401；浏览器侧要调工具，只能改用 API Key 或重新走 Agent 授权。
- 鉴权通过后由中间件链产出 `Scope`，`tenant_id` / `user_id` / `client_id` 只从 `Scope` 取，权限按该用户角色求值。

**会话**

- `initialize` 响应下发 `Mcp-Session-Id`，后续请求带该头。
- 会话状态存 redis：`mcp:session:{tenant_id}:{session_id}`，进程内只保留连接，任意副本都能处理。
- 会话必须与 `(tenant_id, user_id, client_id)` 绑定，任一项不符直接 404。
- 会话空闲 30 分钟过期。

**工具**

- 工具来自 `mcp_tools`（启用状态 + `required_perms` 业务权限点数组），上游地址与凭证来自 `mcp_servers`。
- **权限映射**：`required_perms` 里写的是平台业务权限点（如 `order:read`），不是 MCP 专有权限点。
- `required_perms` 为空 = 登录即可用；非空 = 全部命中才可见、可调用。
- 保存 `mcp_tools` 时校验 `required_perms` 全部在 `authz.All()` 内，写错直接拒绝。
- `tools/list` 只返回 `enabled = true` 且当前用户权限命中的工具；`tools/call` 用**同一组** `required_perms` 再校验一次。
- 求值走与 Web 相同的 `internal/authz` 与同一份角色缓存，用户权限变更后下次 `tools/list` 立即反映（会话不缓存权限集）。
- `tools/call` 执行顺序：

```
解析 tools/call
→ 按 required_perms 逐个 authz.Enforce(obj, act)
→ 配额扣减 mcp_calls
→ 入参按 InputSchema 校验
→ 调上游（超时 30s，响应上限 1MB）
→ 异步写 mcp_call_log
→ 返回 JSON-RPC 响应
```

> 例：Web 端「订单列表 / 订单详情」都对应 `order:read`，则「查询订单」工具配 `required_perms = ["order:read"]`；用户在 Web 里看不到订单，Agent 也拿不到这个工具。

**响应格式**

MCP 端点返回标准 JSON-RPC 2.0，不套统一响应外壳；平台错误码放在 `error.data.code`：

```json
{"jsonrpc":"2.0","id":1,"error":{"code":-32001,"message":"quota exceeded","data":{"code":"30001"}}}
```

| 场景 | `error.code` |
| --- | --- |
| 参数不合法 | `-32602` |
| 方法不存在 | `-32601` |
| 工具无权访问 | `-32003` |
| 上游失败 / 超时 | `-32000` |
| 配额超限 | `-32001` |

**安全**

- 上游 URL 只取 `mcp_servers` 里配置的地址，拒绝私网、回环、云元数据地址。
- 上游凭证加密落库，日志里不出现明文。
- 入参与响应都有大小上限，超限直接报错，不做截断。

**代码接口**

```go
// internal/mcp

type Tool struct {
    Name          string
    Description   string
    InputSchema   json.RawMessage
    RequiredPerms []string // 业务权限点，如 ["order:read"]；空 = 登录即可用
}

// Context 是一次调用的身份上下文，避免业务包依赖 HTTP
// Scope 由中间件写入 ctx，内部用 tenancy.ScopeFrom(ctx) 读取
type Context struct {
    Ctx      context.Context
    TenantID string
    UserID   string
    ClientID string
}

type Provider interface {
    List(ctx Context) ([]Tool, error)
    Call(ctx Context, name string, args json.RawMessage) (json.RawMessage, error)
}

// 挂在租户链路，不套统一响应中间件；由 mcp.Module() 装配并挂到 routes.MCP
func NewHandler(opts Options) *Handler // Options: Provider / Authorizer / Quota / Sessions / Calls
```

---

## 二、目录结构

```
go-saas-lite/
├── cmd/
│   ├── api/main.go              # fx.New + 信号处理 + app.Run
│   ├── worker/main.go           # 复用同一批 provider
│   └── migrate/main.go          # 平台迁移 + 租户迁移执行器
├── internal/                    # 业务：按「平台侧 / 租户侧 / 共享能力」分组
│   ├── app/                     # fx 装配 + 配置加载 + 路由链 + 解析器 + 元数据
│   ├── routes/                  # 路由链分组类型（Routes）：app 构造、模块消费
│   ├── security/                # 共享：认证与授权
│   │   ├── auth/                # JWT、actor 类型、鉴权中间件
│   │   ├── authz/               # 权限点常量、角色求值、缓存 + 失效广播
│   │   ├── apikey/              # 用户级 API Key（生成 / 校验 / 撤销）
│   │   └── oauth/               # OAuth 2.1 服务 + HTTP 适配（单包）
│   ├── mcp/                     # MCP 协议 + 工具网关（会话/JSON-RPC/上游代理/调用日志）
│   ├── quota/                   # 配额接口与校验中间件
│   ├── worker/                  # 维护锁 + 用量落库 + 归档清理
│   ├── platform/                # 平台控制台侧（package platform 聚合子模块）
│   │   ├── module.go            #   聚合 account + tenants + plans + stats + audit
│   │   ├── account/             #   平台会话：登录 / refresh / logout / 改密 / me
│   │   ├── tenants/             #   租户：CRUD + 开通流水线 + 重试 + 用量
│   │   ├── plans/               #   套餐：列表（引用数）/ 新建 / 改配额与启停
│   │   ├── stats/               #   总览：计数 / 趋势 / 用量排行
│   │   └── audit/               #   审计：动作常量 + 写入 + 查询
│   ├── tenant/                  # 租户侧（package tenant 聚合子模块）
│   │   ├── module.go            #   聚合 account + conf
│   │   ├── account/             #   租户用户登录 + API Key 自助
│   │   └── conf/                #   租户配置（DB + 缓存）
│   ├── integration/             # 集成测试用例（真实 PG / Redis）
│   └── testutil/                # 双租户隔离测试夹具（不属生产底座）
├── pkg/                         # 基础设施：只被业务引用，不 import internal
│   ├── apperr/                  # 错误码分段 + 错误类型
│   ├── response/                # code/msg/data、RequestID / ClientIP 中间件
│   ├── obs/                     # slog、otel、metrics、health、recover
│   ├── tenancy/                 # Scope / DB 门面 / Tx / gorm 初始化 / 租户 Callback
│   ├── redisx/                  # 客户端、锁
│   └── migrate/                 # schema_migrations、迁移执行器
├── configs/
├── migrations/
│   ├── embed.go / tables.go     # 迁移嵌入 + 表归属白名单
│   ├── platform/
│   └── tenant/
└── go.mod
```

> 没有 `repo/` 子目录。需要时才加，只放有业务语义的查询。`pkg/` 与 `internal/` 的边界见 D8：`pkg/` 只提供机制，`internal/` 才提供能力。

## 三、架构规则（必须遵守）

### 3.1 分层：两层半

```
handler  →  service  →  tenancy.DB（数据访问门面）
```

| 层 | 职责 | 必须性 |
| --- | --- | --- |
| `handler` | 绑定、校验、统一响应、错误映射。唯一知道 HTTP 语义的层 | **必须** |
| `service` | 业务规则 + **事务边界** + 租户 / 权限编排 | **必须** |
| `tenancy.DB` | 唯一数据库入口：绑 Scope、开事务、交给 Callback 注入 `tenant_id` | **必须** |
| `repo`（struct + interface） | 只在出现真实需求时创建 | **不必需** |

**`repo` 只在以下情况才建**：

1. 同一份查询被 **2 个以上 service** 复用 → 先抽成包级函数，仍不加 interface。
2. 需要**多实现**（本地 / OSS、PG / 缓存双读）→ 在**使用方**定义 interface。
3. 需要跨 transport 复用（HTTP + worker）→ 复用 **service**，不是复用 repo。
4. 需要替换数据源做单测 → 优先用 `testcontainers` 起真 PG，不 mock repo。

**`service` 只持 `tenancy.DB`，不持 `*gorm.DB`。**

### 3.2 隔离实现：`Scope` + GORM Callback（数据库无关）

```go
// pkg/tenancy/db.go —— 全项目唯一的 DB 入口
type DB interface {
    // 开事务；Scope 从 ctx 取，tenant_id 交给 Callback
    Tx(ctx context.Context, fn func(tx Tx) error) error
    // 只读，同样从 ctx 取 Scope
    Read(ctx context.Context, fn func(tx Tx) error) error
}

type Tx interface {
    // 写
    Create(dst any) error                          // 支持单条与切片批量
    Update(dst any, values map[string]any) error   // 按主键或已挂条件更新指定列
    Delete(dst any) error                          // 按主键或已挂条件删除
    DeleteAll(dst any) error                       // 删除当前 Scope 下整表数据（仅租户清理用）

    // 链式条件：返回同一事务内的新句柄，不改动原句柄
    Model(dst any) Tx
    Where(query any, args ...any) Tx
    Order(value any) Tx
    Limit(n int) Tx
    Offset(n int) Tx
    Select(query any, args ...any) Tx

    // 终止操作
    Find(dst any) error
    First(dst any) error
    Count(count *int64) error
    Pluck(column string, dst any) error
    Scan(dst any) error                            // 配合 Raw / Select

    // 原生 SQL：单条参数化查询，租户条件由调用方显式写出（见 3.2.1）。
    // 不提供自由 Joins / 表名拼接入口。
    Raw(sql string, args ...any) Tx
}

// pkg/tenancy/scope.go —— 由中间件写入 ctx，tenancy.DB 从 ctx 读取
type Scope struct {
    TenantID  string
    UserID    string     // 租户用户或 Agent 解析出的用户；平台 Actor 可为空
    ClientID  string     // Web 可为空；OAuth Agent 为 client_id；API Key 可为 apikey:{id}
    Actor     ActorType  // platform | tenant
}

// ScopeFrom 缺失 Scope 时返回错误（fail closed），不允许隐式跨租户。
// 平台身份不带租户，允许 TenantID 为空；租户身份必须带租户。
func ScopeFrom(ctx context.Context) (Scope, error)
```

实现由 GORM Callback 统一完成：

```go
// pkg/tenancy/callback.go
func registerTenantCallbacks(db *gorm.DB) {
    db.Callback().Create().Before("gorm:create").Register("tenancy:set_tenant", setTenantID)
    db.Callback().Query().Before("gorm:query").Register("tenancy:where_tenant", addTenantWhere)
    db.Callback().Update().Before("gorm:update").Register("tenancy:where_tenant", addTenantWhere)
    db.Callback().Delete().Before("gorm:delete").Register("tenancy:where_tenant", addTenantWhere)
}
```

| 环节 | 行为 |
| --- | --- |
| **写（Create / Update）** | 自动填充 `tenant_id`；由 `Scope` 决定，业务不写 |
| **读 / 改 / 删** | 自动追加 `WHERE tenant_id = ?`（GORM Find / Update / Delete 及受控批量操作） |
| **Raw 查询** | 单条参数化 SELECT，`tenant_id = ?` 由调用方显式写出，并配一条双租户隔离用例 |
| **兜底校验** | 普通 GORM 操作缺少租户条件时直接返回错误（开发期可 panic）；`Raw` 不参与自动校验，走 3.2.1 规范 |
| **类型收口** | `*gorm.DB` 不出 `pkg/tenancy`（FX 中 module-private），业务只能拿到 `tenancy.DB`（P0） |
| **隔离测试** | `internal/testutil` 双租户用例覆盖每张业务表（P0） |

关键约定：

- **不设 `Scope.Skip` 开关**。平台侧用另一个连接池 + 另一套 handler 链路（`tenancy.PlatformDB`，不注册租户 Callback）。
- `PlatformTables` 白名单（见 1.3.4）里的表只能经 `tenancy.PlatformDB` 访问，不在租户连接池上做表名豁免。**业务表一旦进白名单，就永久没有隔离。**
- 不依赖数据库特性：隔离只用 `WHERE tenant_id = ?` 这类通用 SQL，与数据库无关。
- 不能用"SQL 里含 `tenant_id`"做安全断言（子查询、别名、`tenant_id = tenant_id` 都会误判），只能当开发期 lint。
- `Raw` 不做 SQL 改写，也不假装能改写：隔离是否成立由「写法规范 + 隔离用例 + 代码评审」保证，不用框架强制。
- 开发期 lint 只能辅助手段，不能替代双租户隔离测试、受控查询 API 与代码评审。
- 业务表建表即要求：`tenant_id NOT NULL` + 独立索引 + `(tenant_id, id)` 复合唯一约束。
- 不允许：跨租户唯一索引、依赖数据库特有隔离能力的表结构。

#### 3.2.1 原生 SQL 写法规范

只在 ORM 表达不了的地方用 `Raw`（分组、聚合、窗口函数、多表统计），并按下面四条写：

1. **只写单条参数化 SELECT**，不拼表名、列名、排序字段；写入、删除、批量更新全部走 ORM。
2. **租户条件显式写在 `WHERE` 里**，值取自 `Scope`，用 `?` 传参，不拼字符串：

```go
const orderSummarySQL = `
SELECT status, COUNT(*) AS total
FROM orders
WHERE tenant_id = ? AND created_at >= ? AND status = ?
GROUP BY status`

func (s *Service) Summary(ctx context.Context, since time.Time) ([]StatusCount, error) {
    scope, err := tenancy.ScopeFrom(ctx)
    if err != nil {
        return nil, err
    }

    var rows []StatusCount
    err = s.db.Read(ctx, func(tx tenancy.Tx) error {
        return tx.Raw(orderSummarySQL, scope.TenantID, since, "paid").Scan(&rows)
    })
    return rows, err
}
```

3. **SQL 常量集中放模块的 `sql.go`**，便于评审，也便于接入其他数据库时逐条改写。
4. **每新增一条 `Raw`，补一条双租户用例**：A 的查询结果里不能出现 B 的数据；用例跟着 SQL 一起提交。

> 框架不会检查 SQL 里是否带了租户条件，也不会替你改 SQL。这条靠规范和评审，隔离用例是最后一道闸。

### 3.3 FX 接线规则

用 `go.uber.org/fx` 组装，必须满足下面 5 条：

1. **`*gorm.DB` 只在 db 模块内可见，两个实例必须打 tag 区分**
   ```go
   fx.Module("db",
       // 同一个 DSN，一个有租户 Callback、一个没有 → 必须打 tag
       fx.Provide(fx.Annotate(dbx.NewTenantDB, fx.ResultTags(`name:"tenant"`))),
       fx.Provide(fx.Annotate(dbx.NewPlatformDB, fx.ResultTags(`name:"platform"`))),

       fx.Provide(fx.Annotate(tenancy.NewDB, fx.As(new(tenancy.DB)))),
       fx.Provide(fx.Annotate(tenancy.NewPlatformDB, fx.As(new(tenancy.PlatformDB)))),
       fx.Private,                                                      // 容器对外不暴露 *gorm.DB
   )
   ```
   > 不打 tag 时两个构造函数会拿到同一个 `*gorm.DB`：编译通过、启动通过，运行时才发现平台池也带了租户 Callback。
2. **接口统一用 `fx.Annotate(ctor, fx.As(new(Iface)))` 注册**
   ```go
   func NewDB(gdb *gorm.DB) *DBProvider        { ... }   // 返回具体类型
   func NewService(db tenancy.DB) *Service     { ... }   // 要接口

   fx.Provide(
       fx.Annotate(NewDB, fx.As(new(tenancy.DB))),
       NewService,
   )
   ```
   - `fx.As` 是**替换**注册类型：容器里只有 `tenancy.DB`，`*DBProvider` 不可注入。
   - 构造函数直接返回接口也可以，就不用 `Annotate`，但会丢失具体类型。
   - `new(Iface)` 只是取接口的 `reflect.Type`。
3. **启动 / 停止顺序写清楚**：`OnStop` 按注册顺序**逆序**执行。HTTP Server 由最后一个 `fx.Invoke` 启动，每个 `OnStop` 用注释标明依赖谁。
4. **CI 必须跑 `fx.ValidateApp(module)`**：依赖图错误是运行时 panic，不是编译错误。
5. **`fx.Invoke` 只做注册**（路由、定时任务、启动 server），不写业务逻辑。

两条附加约束：

- fx 自带日志基于 `zap`：用 `fx.WithLogger` 转接到 `slog`，或全局用 `fx.NopLogger`。
- 依赖图错误在启动期才暴露，靠第 4 条拦。

### 3.4 依赖方向（单向，不许回环）

```mermaid
graph LR
  cmd --> app[internal/app]
  app --> module[各模块 module.go]
  module --> handler
  handler --> service
  service --> tenancyDB[tenancy.DB]
  service --> model[model]
  handler --> tenantctx
  handler --> response
  tenancyDB --> tenancyScope[tenancy.Scope]
```

- `pkg/*` **绝不 import `internal/*`**（用 lint 卡住）
- 每个业务模块导出一个 `Module() fx.Option`，只在该文件里做 `Provide / Invoke`
- 接口**定义在使用方**，不在每个实现包都开 interface
- `model` 是纯数据结构：不写业务方法，不 import 业务包

### 3.5 中间件链顺序（与实现一致）

全局层（`gin.Engine` 上，对所有请求生效）：

```
cors → recover → request_id → [otelgin + trace] → [metrics] → logger
→ /health /readyz /metrics（独立挂载，不进业务链）
```

三条业务入口链分开注册、互不通用（`internal/app/routes.go` 构造 `routes.Routes`，各模块 `Module()` 自己往对应链挂 handler）：

```
租户 Web 链  /api/tenant/v1/*        : auth.Guard(aud=tenant-api, actor=tenant) → TenantStateGuard（状态复查）→ quota.Middleware(QPS + api_calls) → handler
                                      （RBAC 在路由级用 authz.Require(perm) 声明）
平台链       /api/platform/v1/*      : auth.Guard(aud=platform-api, actor=platform) → 路由级 authz.PlatformRequire(perm) → handler
MCP 链       /api/tenant/v1/mcp      : MCPAuth（API Key 或 aud=mcp 的 JWT）→ TenantStateGuard → MCP handler（不套统一响应）
全局登录链   /api/tenant/v1/auth（登录/刷新/登出）: 无租户解析 → handler（登录时查 user_identities 定位租户）
公开链       /api/tenant/v1（OAuth 注册/授权）: TenantResolver(X-Tenant-Id / 子域名，含状态判定) → handler（无令牌）
```

- `cors` 在最外层，目前按 `server.allowed_origins` 静态白名单放行（**尚未按 `tenants.domain/subdomain` 动态解析**）；`OPTIONS` 预检直接返回。
- `auth.Guard` 同时完成「验签 + `aud` 校验 + `actor_type` 校验」（即文档里的 credential_auth + actor_guard），并写入 `Scope`。
- `quota.Middleware` 同时做每秒 QPS 限流（`api_qps`）与 `api_calls` 计数。
- `TenantStateGuard` 承担 `tenant_status_check`：`pending` / `suspended` / `expired` / `archived` 一律拦截，到期为读时判定。
- **尚未实现**：按租户域名动态 CORS（v1 用 `server.allowed_origins` 静态白名单）、`Idempotency-Key` 中间件。见 §九。

### 3.6 事务 / 错误 / 日志

- 事务：**只能在 service 开**，`tenancy.DB.Tx(ctx, fn)` 内部完成 `BEGIN` + 从 ctx 取 `Scope` + `COMMIT`；handler 与 repo 都不许手动 Begin/Commit
- 平台组与租户组是两个连接池，**不存在跨组事务**：需要跨组编排的场景（如开通流水线）按分步提交 + 幂等重试实现，不允许假称原子
- 错误：领域层返回 `*apperr.Error{Code, Msg, Meta, Err}`；handler 层统一 `errors.As` 映射 HTTP，业务代码不出现裸 `c.JSON(500)`
- 日志：统一 `slog.InfoContext(ctx, ...)`，四个关键字段由 `obs` 的 Handler 自动补全

### 3.7 禁止清单

- handler 里出现 `gorm.DB`、或直接操作数据库
- service 持有 `*gorm.DB`（只能持 `tenancy.DB`）
- 业务代码里手写 `tenant_id` 条件
- 为每个表建只做转发的 Repository
- 全局变量持有 `*gorm.DB` / `*redis.Client` / logger（`obs` 默认 handler 除外）
- 在包的 `init()` 里做 FX 注册或读配置
- `pkg/` 里 `os.Exit`、直接读环境变量（配置由 `cmd` 注入）
- 用 `AutoMigrate` 跑生产迁移
- 定时任务裸 `time.Ticker`（多副本会重复执行）

## 四、隔离方案

| 方案 | 状态 | 要求 |
| --- | --- | --- |
| 共享库 + 共享表 + `tenant_id` | **默认** | 必须靠 `Scope` + GORM Callback + 隔离测试（见 3.2） |
| 共享库 + 独立 Schema | 扩展 | 框架只给 `TenantSchemaManager` 接口，另需"存量租户批量补迁移"执行器 |

## 五、技术选型

| 层面 | 选型 |
| --- | --- |
| Web | **gin**  |
| ORM | **gorm.io/gorm v2** |
| 迁移 | **goose** / **atlas** |
| DI | **go.uber.org/fx** |
| 隔离 | **GORM Callback** + **`Scope`** |
| 授权 | **自建**：`roles` + `user_roles` + 权限点常量 |
| 配置 | **koanf** |
| 日志 | **slog** |
| 校验 | **go-playground/validator** |
| JWT | **golang-jwt/jwt v5** |
| Redis | **redis/go-redis v9** + **bsm/redislock** |
| ID | **google/uuid** |
| 观测 | **OpenTelemetry** + otelgin + prometheus exporter |
| 队列 | **asynq**（redis）— **未接入** |
| 测试 | **testify** + **testcontainers-go** |
| 错误 | 标准库 **errors**（`errors.Is` / `errors.As`） |
| 文档 | **huma** — **未接入** |
| MCP | **modelcontextprotocol/go-sdk** |

## 六、必查清单

1. **平台与租户身份串号**：同一个人可能既是平台管理员又是某租户用户 → `actor_type` + `aud` 必须双校验，且 `aud` 要区分 `tenant-api` / `mcp` / `platform-api`
2. **超管隔离**：平台侧使用独立账号 + 独立连接池（不注册租户 Callback），不与租户侧共用实现，也不使用 `Skip` 布尔开关
3. **协程 / 队列上下文传递**：`context.WithoutCancel(ctx)` 派生 + payload 显式带 `tenant_id`（当前无 `tenantctx` 包）
4. **缓存串租户**：redis key 统一 `tenant:{tid}:xxx` 前缀，由框架封装，业务不能自拼
5. **配置热更一致性**：多副本必须广播或版本号校验
6. **迁移可重入**：新增业务表要能同步到所有存量租户（Schema 模式）
7. **租户删除**：软删 → 冷却期 → 异步清理，清理逻辑业务实现
8. **时区 / 语言**：`tenants.timezone` / `locale`，影响日志、定时任务、报表
9. **错误码不串段**：框架 1~4 段，业务 9 段起 
10. **优雅关闭**：容器滚动更新时不丢在途请求
11. **租户用户登录的前置条件**：`users` 在隔离区，登录必须先解析出 `tenant_id`，否则查不到人（见 3.3）
12. **最后一个管理员保护**：删除用户 / 解绑角色前，必须校验该租户仍有 ≥1 个 active 且持 `admin` 的用户，否则租户会把自己锁死
13. **角色权限变更同步**：多副本下改角色权限 / 绑定后必须广播 `authz:reload:{tenant_id}` 失效本地缓存，否则出现"权限时好时坏"
14. **表归属双向测试**：`migrations/platform` 的表全部在 `PlatformTables` 白名单内，`migrations/tenant` 的表一个都不在；否则有人把业务表建到平台组（永不隔离）
15. **权限点必须是代码常量**：写入 `roles.permissions` 或 `mcp_tools.required_perms` 前都要校验在 `authz.All()` 内，禁止前端直接提交权限字符串
16. **内置角色权限靠迁移同步**：`cmd/migrate` 的 `SyncBuiltinRoles` 每次发版必跑，否则老租户拿不到新增权限点
17. **MCP 不套统一响应**：MCP 端点返回 JSON-RPC 2.0，平台错误码放 `error.data.code`
18. **MCP 会话必须绑租户与用户**：`Mcp-Session-Id` 存 redis 且带 `tenant_id` / `user_id` / `client_id`，与当前 token 不一致直接 404
19. **工具可见性与可调用性同源**：`tools/list` 与 `tools/call` 用同一组 `required_perms`，不能只在 list 上过滤
20. **上游地址白名单**：上游 URL 只取 `mcp_servers` 里的地址，做 SSRF 校验，拒绝私网 / 回环 / 云元数据地址
21. **工具调用必须计量**：`tools/call` 扣 `mcp_calls` 配额 + 异步写 `mcp_call_log`，不阻塞响应
22. **MCP 会话计数入配额**：`mcp_sessions` 按租户并发会话数校验，会话建立时占位、过期时释放
23. **Agent 身份必须是用户级**：OAuth token 的 `sub` 与 API Key 都落到 `user_id`，MCP 按该用户角色求值
24. **授权码单次使用 + PKCE S256**：code 存 redis、10 分钟过期、用后即删；`code_verifier` 校验不过一律拒绝
25. **CORS 白名单**：v1 用 `server.allowed_origins` 静态白名单（按租户域名动态放行属于子域名模式，v1 不启用），不允许 `*`，`OPTIONS` 预检不进鉴权链
26. **refresh token 只存哈希**：redis 里存 sha256，登出或撤销授权时删除
27. **撤销授权即时生效**：`oauth_grants.revoked_at` + 删 refresh + 作废 `Mcp-Session-Id`，三件事同一请求内完成
28. **API Key 只存哈希**：库里只存 `sha256(key)` 与前缀，明文只在创建响应里返回一次
29. **API Key 必须带人与租户**：`api_keys` 同时有 `tenant_id` 与 `user_id`，鉴权后 `Scope` 缺 `user_id` 一律拒绝
30. **租户以令牌为准**：已鉴权链的租户一律取令牌里的 `tenant_id`；`X-Tenant-Id` 只是前端的「当前选择」声明，与令牌不一致时以令牌为准并记录告警，不允许任何一方覆盖身份
31. **MCP 与 Web 权限同源**：不建 MCP 专有角色或权限点，MCP 与 Web 共用同一个 `Authorizer` 和同一份角色缓存
32. **会话不缓存权限集**：`Mcp-Session-Id` 只存身份与连接状态，每次 `tools/list` / `tools/call` 重新取权限，改权限后无需重连
33. **权限点只到资源级**：不出现 `order:list` / `order:detail` 这类同义拆分，不按接口或按钮建点；4 个标准动作之外的动作必须用 `authz.Custom` 显式定义
34. **MCP 只认 Agent 凭证**：MCP 端点只接受 `aud = mcp` 的 OAuth token 与用户级 API Key；Web 的 `aud = tenant-api` JWT 一律拒绝，避免浏览器令牌被复用到工具调用
35. **开通流水线不做跨组事务**：`tenants` 与租户组表分属两个连接池，按「平台段提交 → 租户段事务 → 步骤落库」推进，`provision_step` 必须独立提交
36. **`Scope` 只有一份来源**：由中间件写入 ctx，`tenancy.DB` 与业务方法一律经 `tenancy.ScopeFrom(ctx)` 读取；缺失即报错，不隐式跨租户
37. **原生 SQL 自带租户条件**：`Raw` 只写单条参数化 SELECT，`tenant_id` 取自 `Scope`，且该查询必须配一条双租户隔离用例；不依赖框架改写 SQL

## 七、快速开始、回归与交付顺序

### 7.1 快速开始

前置：Go 1.27+、PostgreSQL 14+、Redis 6+、Node 18+（仅前端 / 门户构建需要）。

```powershell
# 1. 准备数据库（示例）
#    createdb saas_test

# 2. 执行迁移（平台组 + 租户组）
go run ./cmd/migrate -dsn "postgres://postgres:123456@127.0.0.1:5432/saas_test?sslmode=disable" -command up -group all

# 3. 初始化平台管理员
go run ./cmd/migrate -dsn "postgres://postgres:123456@127.0.0.1:5432/saas_test?sslmode=disable" -command up -group platform -platform-admin admin@example.com:password123

# 4. 启动 API（dev 下会同时挂 worker 与队列消费者）
go run ./cmd/api

# 5. 打开文档与门户
#    http://localhost:8080/docs
#    http://localhost:8080/
```

前端控制台：

```powershell
npm --prefix web install
npm --prefix web run dev
```

常用配置（优先用 `SAAS_` 环境变量覆盖 `configs/config.yaml`）：

| 变量 | 说明 |
| --- | --- |
| `SAAS_DATABASE__PLATFORM_DSN` / `SAAS_DATABASE__TENANT_DSN` | 平台组 / 租户组连接串 |
| `SAAS_REDIS__ADDR` | Redis 地址；为空时关闭缓存、队列与失效广播 |
| `SAAS_STORAGE__DRIVER` | `local` / `cos` / `oss` |
| `SAAS_STORAGE__COS__SECRET_ID` / `SAAS_STORAGE__COS__SECRET_KEY` | 腾讯云 COS 凭据 |
| `SAAS_STORAGE__OSS__ACCESS_KEY_ID` / `SAAS_STORAGE__OSS__ACCESS_KEY_SECRET` | 阿里云 OSS 凭据 |
| `SAAS_QUEUE__ENABLED` | 是否启用 asynq |
| `SAAS_SMS__PROVIDER` | 当前仅 `dev`（验证码写日志） |
| `SAAS_PORTAL__AUTO_APPROVE` | 自助注册是否自动开通 |

全量测试：

```powershell
$env:TEST_DATABASE_DSN="postgres://postgres:123456@127.0.0.1:5432/saas_test?sslmode=disable"
$env:TEST_REDIS_ADDR="127.0.0.1:6379"
go test ./...
```

> 测试共用一个库，用例已按唯一邮箱 / 手机号隔离；不要并行跑多个 `go test ./...` 进程。

### 7.2 手工回归清单

1. 门户注册：`/signup` 提交 → 收到验证码（dev 看日志）→ 完成注册；`auto_approve` 关闭时租户为 `pending`。
2. 租户登录：邮箱 + 密码、手机号 + 密码、手机号 + 验证码三种方式；连续失败 5 次触发锁定。
3. 强制改密：新开通 / 被重置密码的账号登录后只能访问 `/me` 与 `/auth/password`。
4. 成员：创建（邮箱或手机号）、停用最后一个管理员被拒绝、变更角色、重置密码。
5. 配额：把套餐 `max_users` 调小后创建成员返回配额错误；`/plan` 与总览显示一致。
6. 文件：上传 → 列表 → 签名 URL 下载 → 删除；跨租户读取返回 404。
7. 租户审计：完成上述操作后 `GET /api/tenant/v1/audit-logs` 能看到对应 `action`，另一个租户看不到。
8. MCP：`initialize` → `tools/list` → `tools/call`；非法 `arguments` 被 `InputSchema` 拒绝；撤销授权后旧会话失效。
9. 平台侧：开通租户 → 查看进度时间线 → 重试 / 重置密码 / 归档；套餐启停与审计可查。
10. worker：归档租户超过冷却期后，框架表、`tenant_files` 与对象存储文件均被清理。

### 7.3 交付顺序（里程碑）

| 阶段 | 内容 |
| --- | --- |
| M1 | `cmd/api` 的 fx 骨架 + `pkg/response` + 错误码 + `pkg/obs` + `/health` `/readyz` `/metrics` + 优雅关闭 |
| M2 | **最小测试迁移 + `pkg/tenancy`（Scope + DB 门面 + Tx）+ GORM Callback + `internal/testutil` 双租户隔离用例**；`Tx` 接口在本阶段一次冻结，最小迁移只验证隔离门面和表归属，完整迁移器留给 M4 |
| M3 | `internal/security/auth`（两套 actor）+ `internal/security/authz`（角色求值 + 缓存失效）+ 两条路由链 + `actor_guard`（已内置在 `auth.Guard` 的 aud+actor 双校验）+ `permission` 中间件（路由级 `authz.Require`） |
| M4 | `pkg/migrate` + 平台 / 租户迁移 + `cmd/migrate` + 存量租户批量迁移 |
| M5 | Provisioning 流水线（含初始管理员 + 内置角色授予）→ 租户 CRUD + 租户配置 |
| M6 | 配额校验 + `cmd/worker`（注销清理、用量落库） |
| M7 | `internal/security/oauth`（授权码 + PKCE + 动态注册）+ `internal/mcp`（Streamable HTTP + 会话 + JSON-RPC + 工具网关，已合并原 mcpgateway）+ MCP 配额与调用日志 |
| M8 | 平台控制台：会话（email 登录 / me / refresh / logout / 改密）+ owner|viewer 权限矩阵 + 租户管理全链路（列表 / 开通 / 进度时间线 / 重试 / 续费 / 归档 / 重置密码 / 用户 / 配额用量）+ 套餐 + 审计 + 总览 + 到期治理 |
| M9 | 租户控制台：**全局登录**（邮箱定位租户 + 平台组 `user_identities` 索引 + 对账）+ 会话（refresh 轮换 / logout / 改密 / `must_change_password` 强制）+ 成员与角色（一次性初始密码、最后管理员保护）+ 总览 + MCP 服务 / 工具 / 调用日志 + API Key + Agent 授权撤销 |

> M1–M9 已落地；平台与租户两套控制台均已切真实接口。

## 八、下一步

1. 租户侧安全收尾：密码复杂度策略、`token_version` 在 Guard 中强制校验（当前仅文档化范围）
2. MCP 协议收尾：`MCP-Protocol-Version` 头协商、`/oauth/consent` 同意页
3. 写接口幂等（`Idempotency-Key`）、pprof 独立内网端口、CORS 按租户域名动态白名单
4. 平台运行时配置（D10 第二层）落库；当前仍用 `configs/config.yaml` + 环境变量
5. Schema-per-tenant（`TenantSchemaManager`、`cmd/migrate --all-tenants`）
6. 计费与导出（下一阶段业务平台，当前仅预留套餐/用量数据）

## 九、实现现状核对（未完成清单）

> 本节按 2026-11-07 代码实际状态核对。上方正文是**目标设计**；本节列出与设计的差距，某项实现后请同步删除对应条目。

### 9.1 已完成（有实现，多数有测试）

- **租户生命周期**：开通（平台段提交 → 租户段事务 → 步骤落库）、CRUD、状态机（`canTransition`）、归档；迁移含 `tenants`/`roles`/`user_roles`/`users`/`tenant_configs`。
- **隔离**：`Scope` + GORM Callback + `PlatformDB` 双连接池 + `platformTables` 白名单 + 双租户隔离用例。
- **鉴权**：JWT（HS256，`aud` + `actor_type` 双校验，`typ` 区分 access / refresh）、平台/租户/MCP 三条链、平台路由级 `authz.PlatformRequire(perm)`（owner / viewer 静态矩阵）、`TenantStateGuard` 状态复查、`must_change_password` 强制改密守卫。
- **平台控制台**：会话（email 登录 / `GET /me` / refresh / logout / 改密）、租户管理（列表筛选 / 开通 / 进度时间线 / 重试 / 续费 / 归档 / 重置密码 / 用户 / 配额用量）、套餐（列表 + 引用数 / 新建 / 改配额 / 启停）、审计（6 分类 + 动作常量 + 风险级别 + 保留期清理）、总览（计数 / 近 7 天趋势 / 用量 Top5 / 失败待处理）。
- **到期治理**：读时判定（`active` + `expire_at` 已过 → `expired`，不写库）+ 公开链与已鉴权链状态复查 + worker `ExpireTenants` 落库。
- **租户控制台**：**邮箱 / 手机号全局登录**（平台组 `user_identities` 索引定位租户，`cmd/migrate -command repair-identities` 对账）+ 验证码登录 / 注册 / 重置密码（`internal/security/otp`，短信通道 `pkg/sms` 当前为 dev 日志）+ 登录失败锁定（5 次 / 15 分钟）+ 会话（refresh cookie 轮换、logout、改密、强制改密）+ `GET /me` + 成员与角色（邮箱/手机号至少一个、一次性初始密码、状态启停、角色替换、重置密码、最后管理员保护）+ 总览 + MCP 配置 + API Key + Agent 授权列表 / 撤销。
- **RBAC**：权限点常量、内置角色（admin/member）、进程内缓存 + redis pub/sub 失效、`SyncBuiltinRoles`。
- **配额**：`plans` + Redis 用量 + worker 落 `usage_daily` + QPS/`api_calls` 中间件 + MCP `mcp_calls` / `mcp_tokens` / `mcp_sessions` 计量 + `max_users` 创建校验。
- **租户配置**：`tenant_configs` 读写 + 本地缓存 + Redis `tenantconf:reload` 失效广播。
- **文件存储**：`pkg/storage`（local / cos / oss）+ `tenant_files` 元数据 + 上传/列表/删除/签名 URL + `storage_gb` 配额 + 归档清理钩子。
- **审计**：平台 `system_operation_log` 与租户 `tenant_operation_log` 双通道，均支持分类 / 动作 / 级别 / 操作人 / 时间范围查询。
- **OAuth 2.1**：register / authorize / token / refresh（轮换）/ revoke + well-known + PKCE S256 + 单次授权码；撤销授权时同步作废 MCP 会话。
- **API Key**：`sk_live_` 生成 / 列表 / 撤销 / 鉴权（sha256 存储）。
- **MCP**：Streamable HTTP 单端点、会话（Redis / 内存）、JSON-RPC、`tools/list` + `tools/call`、`InputSchema` 入参校验、按 `required_perms` 过滤与再校验、用量扣减 + 异步 `mcp_call_log`、上游代理 + SSRF 校验、SSE 单帧。
- **异步任务**：asynq + `pkg/queue` + `internal/tasktype`；已注册 `sms.send`、`file.delete`、`tenant.purge`、`tenant.provision.retry`、`demo.heartbeat`；优雅关闭，未启用时自动降级。
- **营销门户**：Go 模板 + Tailwind CSS，提供首页 / 定价 / 注册（含 OTP）/ 登录 / 条款 / 隐私；同源校验 + IP 限流 + 蜜罐，支持审核制与自动开通。
- **API 文档**：OpenAPI 3.1（`docs/api/openapi.yaml`）+ `/openapi.yaml` + `/docs` Swagger UI，并有路由与规范双向漂移测试。
- **迁移**：goose + checksum + advisory lock + `cmd/migrate` 的 `up/status/verify/sync-roles/platform-admin`。
- **worker**：redis 维护锁、用量落库、到期流转、归档清理（含文件对象）、审计保留期清理；dev 下随 api 启动。
- **组件**：response / apperr / obs（slog/otel/metrics/health/recover）/ 优雅关闭 / koanf 配置。

### 9.2 未完成 / 与设计有差距

| # | 项目 | 现状 | 影响 / 备注 |
|---|---|---|---|
| 1 | 密码复杂度策略 | 未实现 | 仅校验长度 ≥ 8；登录失败锁定已完成 |
| 2 | `token_version` 在 Guard 校验 | 未实现 | 改密 / 重置时递增，但已签发 access token 不会立即失效；当前仅文档化范围（H5） |
| 3 | `MCP-Protocol-Version` 头协商 | 未实现 | `initialize` 固定回传服务端默认版本 |
| 4 | `/oauth/consent` 同意页 | 未实现 | `authorize` 直接签发授权码 |
| 5 | Schema-per-tenant（`TenantSchemaManager`、`cmd/migrate --all-tenants`） | 未实现 | 仅共享表模式；`--all-tenants` 未启用即报错 |
| 6 | 写接口幂等（`Idempotency-Key`） | 未实现 | 只在 CORS 允许头里列了该头，无中间件 |
| 7 | pprof 独立内网端口 | 未实现 | 未挂载 |
| 8 | CORS 按租户域名动态白名单 | 未实现 | 目前用 `server.allowed_origins` 静态配置 |
| 9 | 平台运行时配置（D10 第二层，DB） | 不落库 | 使用进程配置（yaml + env）；`audit.retention_days` 等调整改配置后重启生效 |
| 10 | 集成测试目录 `internal/integration/` | 未建立 | 需要真实库的用例仍与包同目录 |
| 11 | 短信云通道 | 仅 dev | `sms.provider=dev` 只写日志；阿里云 / 腾讯云发送器未接 |
| 12 | 租户二级域名 / Host 解析 | 可选能力 | 登录已改为全局标识登录；`tenancy.base_domain` + `TenantResolver` 仍供 OAuth 与自定义域名场景使用 |