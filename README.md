# go-saas-lite

一个轻量、单体部署的 Go SaaS 底座：把租户隔离、账号鉴权、套餐配额、文件存储、营销门户、异步任务、审计与 MCP 接入这些“重复造轮子”的部分收在一套可运行的项目里，业务系统直接在此基础上加模块。

## 特性

- **多租户隔离**：共享库 + 共享表 + `tenant_id`，由 `Scope` + GORM Callback 自动注入；平台侧使用独立连接池，不注册租户 Callback。
- **租户生命周期**：开通流水线、状态机、到期治理、归档与冷却期清理。
- **两套身份体系**：平台管理员与租户用户物理隔离；JWT 按 `aud` 区分 `platform-api` / `tenant-api` / `mcp`。
- **租户登录**：邮箱 + 密码、手机号 + 密码、手机号 + 验证码；全局登录索引 `user_identities`，登录无需先知道租户。
- **RBAC**：内置 `admin` / `member` 角色、代码权限点、进程内缓存 + Redis 失效广播、最后管理员保护。
- **套餐与配额**：`max_users`、`storage_gb`、`api_qps`、`api_calls`、`mcp_calls`、`mcp_tokens`、`mcp_sessions`。
- **文件存储**：统一 `Storage` 接口，内置本地磁盘、腾讯云 COS、阿里云 OSS 驱动；文件元数据落 `tenant_files`，支持签名下载。
- **营销门户**：Go 模板 + Tailwind CSS，含首页、定价、注册（短信验证码）、登录跳转、条款与隐私页。
- **异步任务**：asynq 封装 + 定时调度器；内置短信、文件删除、租户清理、开通续跑等任务。
- **审计**：平台 `system_operation_log` 与租户 `tenant_operation_log` 双通道。
- **MCP 网关**：Streamable HTTP + JSON-RPC 2.0，支持会话、工具注册、`InputSchema` 校验、权限过滤与上游代理。
- **OAuth 2.1**：动态注册、授权码 + PKCE S256、refresh 轮换、撤销授权并作废 MCP 会话。
- **API 文档**：OpenAPI 3.1 规范 + Swagger UI，路由与规范有漂移测试。
- **单体轻量**：不依赖 CI/CD、Docker、K8s，一个 API 进程 + 一个可选 worker 进程即可运行。

## 技术栈

Go 1.27+ · gin · gorm v2 · PostgreSQL · Redis · goose · fx · koanf · slog · asynq · OpenTelemetry · Prometheus

前端控制台：Vue 3 + Vite + Element Plus + Pinia。

## 快速开始

### 1. 前置条件

- Go 1.27+
- PostgreSQL 14+
- Redis 6+
- Node.js 18+（仅前端控制台开发需要）

### 2. 准备配置

```powershell
Copy-Item configs/config.example.yaml configs/config.yaml
```

`configs/config.yaml` 是本地配置，**不要提交**。生产环境优先通过 `SAAS_` 前缀的环境变量覆盖，例如：

```powershell
$env:SAAS_DATABASE__PLATFORM_DSN="postgres://user:pass@127.0.0.1:5432/saas?sslmode=disable"
$env:SAAS_DATABASE__TENANT_DSN="postgres://user:pass@127.0.0.1:5432/saas?sslmode=disable"
$env:SAAS_REDIS__ADDR="127.0.0.1:6379"
$env:SAAS_AUTH__SECRET="replace-with-a-long-random-secret"
```

### 3. 初始化数据库

```powershell
go run ./cmd/migrate -dsn "postgres://user:pass@127.0.0.1:5432/saas?sslmode=disable" -command up -group all
go run ./cmd/migrate -dsn "postgres://user:pass@127.0.0.1:5432/saas?sslmode=disable" -command up -group platform -platform-admin admin@example.com:password123
```

### 4. 启动 API

```powershell
go run ./cmd/api
```

- Swagger UI：<http://localhost:8080/docs>
- 健康检查：<http://localhost:8080/health>
- 营销门户：<http://localhost:8080/>

开发环境（`app.env=dev`）下，API 进程会同时启动 worker 与 asynq 消费者；生产环境建议单独运行：

```powershell
go run ./cmd/worker
```

### 5. 启动前端控制台

```powershell
npm --prefix web install
npm --prefix web run dev
```

## 配置说明

参考 `configs/config.example.yaml`。常用项：

| 配置 / 环境变量 | 说明 |
| --- | --- |
| `database.platform_dsn` / `database.tenant_dsn` | 平台组 / 租户组连接串，可指向同一个库 |
| `redis.addr` | Redis 地址；为空时关闭缓存、队列与失效广播 |
| `auth.secret` | JWT 签名密钥；生产必须通过环境变量注入 |
| `storage.driver` | `local` / `cos` / `oss` |
| `sms.provider` | 当前仅 `dev`（验证码写日志） |
| `queue.enabled` | 是否启用 asynq |
| `portal.auto_approve` | 自助注册是否自动开通租户 |
| `docs.enabled` | 是否暴露 `/openapi.yaml` 与 `/docs` |

## 项目结构

```text
cmd/                 api / worker / migrate 入口
internal/app/        fx 装配、配置、路由链、租户解析
internal/platform/   平台控制台：账号、租户、套餐、统计、审计
internal/tenant/     租户控制台：账号、成员、配置、文件、审计、统计
internal/security/   JWT、RBAC、API Key、OAuth、OTP、登录限流
internal/mcp/        MCP 会话、JSON-RPC、工具网关
internal/apidocs/    OpenAPI 规范与 embed
internal/quota/      套餐限额与用量
internal/worker/     定时维护任务与 asynq 消费者
pkg/                 基础设施：tenancy、response、obs、queue、storage、sms、migrate
migrations/          platform / tenant 两组迁移
web/                 Vue 3 控制台 + 门户静态资源
```

## 测试

需要真实 PostgreSQL 与 Redis：

```powershell
$env:TEST_DATABASE_DSN="postgres://user:pass@127.0.0.1:5432/saas_test?sslmode=disable"
$env:TEST_REDIS_ADDR="127.0.0.1:6379"
go test ./...
```

测试共用一个库，用例通过唯一邮箱 / 手机号隔离；不要并行运行多个 `go test ./...` 进程。

## 当前状态

底座主体已完成，可直接作为业务系统起点。已实现：租户生命周期与隔离、平台/租户控制台、JWT 与 RBAC、套餐配额、文件存储、门户注册、OTP 登录、异步任务、双通道审计、MCP、OAuth 2.1、OpenAPI 文档。

尚未完成或需要按部署环境补齐：

- 真实短信通道（当前 `dev` 只写日志）与短信模板配置
- 上传 MIME 白名单
- 租户控制台前端的手机号登录、文件页、审计页
- access token 立即失效（`token_version` 在 Guard 校验）
- `Idempotency-Key`、pprof、动态 CORS、平台运行时配置落库
- MCP 协议版本协商、OAuth 同意页

完整设计说明、数据设计与迭代计划在本地 `docs/` 中维护，未纳入本仓库。

## 说明

本项目定位为单体、轻量的 SaaS 底座，明确不包含 CI/CD、Dockerfile、Compose、K8s 与微服务拆分。
