# Aetheris 服务端架构设计

**日期：** 2026-09-06  
**状态：** 待实现  
**范围：** 仅服务端：Go Server、PostgreSQL、Admin Web、Python Model Gateway、部署与测试。

## 1. 范围与边界

本次改造将当前 Python Gateway 替换为模块化 Go Server。服务端负责认证 API、事件校验与接收、PostgreSQL 持久化、终端/设备管理、工作角色分配、审计日志、导出，以及 Admin Web 静态文件托管。

Windows Core 和本地 Lens 不在本线程实现。它们现有行为作为兼容契约保留：Core 上传已脱敏的 `AetherisEvent`，Lens 消费仅限本机的本地 API。服务端改造必须保持这些契约，或通过版本化 endpoint 扩展。

本线程的服务端交付物：

- `server/`：Go module，以及 `aetheris-server`、`aetheris-migrate` 两个二进制入口。
- `admin-web/`：React + TypeScript + Vite 独立静态前端。
- `model-gateway/`：Python 服务，提供 provider-neutral 的 Ollama/Dify 适配器。
- `deploy/`：本地 Docker Compose，以及 Linux 生产部署的 systemd/安装资产。
- `contracts/`：JSON Schema、OpenAPI 和生成的 TypeScript 类型。

## 2. 运行拓扑

```text
Windows Core（现有客户端）
      | HTTPS device token，仅出站
      v
Go Server :8080
  /api/v1/*       认证 API
  /admin/*        Admin Web 静态资源
      | SQL
      v
PostgreSQL
      |
      +--> 可选对象存储适配器
      +--> 可选向量索引适配器
      +--> 内部 HTTP --> Python Model Gateway
                              |--> Ollama
                              `--> Dify
```

Go Server 不导入 Admin Web 或 Model Gateway 的实现代码。Admin Web 不直接连接 PostgreSQL。Model Gateway 不连接 PostgreSQL，只接收 Go Server 按权限筛选后的最小事件投影。

## 3. 数据库模型

PostgreSQL 是生产环境的唯一事实源。所有变更通过版本化 migration，由 `aetheris-migrate` 执行。

### 3.1 身份与访问权限

- `tenants`：组织/租户边界。
- `users`：Admin 登录用户；密码使用 Argon2id 哈希。预留 OIDC subject 标识字段。
- `access_roles`、`permissions`、`access_role_permissions`：系统访问角色和稳定权限字符串。
- `memberships`：用户到租户的访问角色绑定。
- `project_memberships`：项目级访问角色覆盖。
- `api_sessions`：刷新/会话 token 的哈希、过期时间和撤销时间。

访问角色与被采集用户的业务工作角色完全分离。第一版访问角色为：`platform_admin`、`tenant_admin`、`analyst`、`reviewer`、`member`、`device_ingest`。

### 3.2 被采集主体、终端和工作角色

工作角色表示被采集用户的业务身份，不表示其使用 Admin 的权限：

- `subjects`：一个被采集用户主体。一个主体可以拥有多个终端。
- `devices`：一个 Windows 终端；第一版一个终端只绑定一个主体。记录稳定设备 ID、客户端版本、主机名、状态和最后心跳时间。
- `device_credentials`：设备 token 哈希、scope、创建/撤销时间和最后使用时间。
- `work_roles`：租户级工作角色字典，例如 `研发`、`测试`、`产品`；包含稳定 code、显示名称、版本和启用状态。
- `work_role_assignments`：主体级或终端级角色分配；支持项目覆盖、有效时间区间、来源和确认状态。

一个主体可以有多个终端；一个终端只对应一个主体。工作角色生效优先级为：项目覆盖 > 主体分配 > 租户默认值。事件入库时固化工作角色快照（`role_id`、`code`、`version`、`source`），角色后续变更不会修改历史统计结果。

### 3.3 事件与派生数据

- `projects`：租户所属项目根目录及元数据。
- `events`：不可变的已脱敏事件信封；使用结构化列加 `payload JSONB`；`event_id` 全局唯一；按租户、项目、设备、主体、工作角色、类型、时间建立索引。
- `event_tombstones`：删除原因和时间；阻止已删除 ID 被重放。
- `event_blobs`：可选对象存储引用（`provider`、`object_key`、`content_hash`、`size_bytes`、保留策略）。
- `event_embeddings`：可选向量 provider 引用和 embedding 模型版本。
- `model_runs`：provider、模型、任务、状态、耗时、错误码和输入事件 ID；默认不保留原始 prompt/response。
- `audit_logs`：追加写入的 actor、动作、资源、范围和结果；不写入 payload 和凭据。

所有租户数据表都必须包含 `tenant_id`；项目级数据表还必须包含 `project_id`。Go repository 强制要求传入租户范围，PostgreSQL 启用 Row-Level Security 作为第二层防护。

## 4. API 与认证

### 4.1 设备 API

```text
POST /api/v1/device/bootstrap
POST /api/v1/device/heartbeat
POST /api/v1/ingest
```

Bootstrap 使用一次性注册密钥并返回设备 token。服务端只保存 token 哈希。Heartbeat 和 ingest 的 `tenant_id`、`subject_id`、`device_id` 从凭证反查得到；客户端提交的身份字段被忽略，或在不一致时拒绝请求。

### 4.2 Admin API

```text
POST /api/v1/auth/login
POST /api/v1/auth/refresh
POST /api/v1/auth/logout
GET  /api/v1/admin/me
GET  /api/v1/admin/devices
GET  /api/v1/admin/subjects
GET  /api/v1/admin/work-roles
PUT  /api/v1/admin/work-role-assignments
GET  /api/v1/admin/events
POST /api/v1/admin/events/{id}/tombstone
GET  /api/v1/admin/audit-logs
POST /api/v1/admin/model-runs
GET  /api/v1/admin/health
```

Admin Web 使用 `HttpOnly`、`SameSite` 安全 cookie 保存会话；所有 cookie 认证的变更请求必须有 CSRF 防护。权限统一由 `Authorizer` 服务根据 permission 加租户/项目 scope 判断，handler 内不允许散落自定义角色判断。

### 4.3 兼容契约

现有 `AetherisEvent` JSON Schema 继续作为 ingest 契约。服务端可以增加可选的 `work_role` 快照字段和版本化 endpoint，但迁移期间必须继续接受合法的 schema-v1 事件。OpenAPI 和生成的 TypeScript 类型是 Admin Web API client 的唯一来源。

## 5. Model Gateway

Python 服务可选启用、独立部署，只暴露一个带内部认证的 endpoint：

```http
POST /internal/v1/generate
{
  "tenant_id": "tenant-...",
  "actor_id": "user-...",
  "task": "summarize",
  "model": "default",
  "input_event_ids": ["event-..."],
  "context": {"work_role": "研发"},
  "messages": []
}
```

Python 定义 `ModelProvider` protocol。`OllamaProvider` 默认调用本机 Ollama HTTP API；`DifyProvider` 使用可配置的 Dify API URL、key 和 workflow/app 标识。provider 选择、超时、重试、响应大小限制和结构化输出校验集中在 gateway 内实现。Go Server 负责 RBAC 筛选，只发送最小必要投影；Python 服务不能自行查询数据库。

## 6. Admin Web

Admin Web 是独立的 React + TypeScript + Vite 项目。它使用生成的 API 类型，支持登录/会话刷新、设备清单、主体与工作角色分配、按项目/设备/主体/工作角色过滤事件、提交 tombstone、查看审计日志和查看模型运行状态，并明确处理 loading、empty、unauthorized、server-error 状态。

`npm run dev` 通过 API proxy 独立运行前端；`npm run build` 生成静态资源，再复制到 Go Server 配置的 `web/admin` 目录。前端不实现业务授权，服务端始终是权限事实源。

## 7. 部署与存储

生产根目录为配置服务器上的 `/opt/aetheris`。凭据只能通过外部 secret 机制或 SSH agent 注入；任何服务器密码、token 都不得提交、写入日志或嵌入脚本。

```text
/opt/aetheris/
  bin/aetheris-server
  bin/aetheris-migrate
  web/admin/
  config/server.env        # 权限 0600
  backups/
  logs/
```

初始空间规划为：PostgreSQL 25 GB、对象存储 40 GB、向量索引 15 GB、模型/Dify 数据 10 GB、日志/备份/升级空间 10 GB。服务启动时检查 migration 版本、数据库连通性、可选 provider 健康状态和磁盘阈值。必须启用日志轮转和备份保留策略；日志不得包含事件 payload、凭据、原始 prompt 或模型 response。

## 8. 故障与安全行为

- ingest 以 `event_id` 幂等；重复请求不能产生第二条事件。
- 非法事件按字段返回拒绝原因，客户端可以保留并检查。
- PostgreSQL 或模型 provider 临时故障不能静默丢弃已经接收的客户端事件。
- 模型 provider 故障写入 `model_runs`，不能阻塞事件 ingest。
- 已 tombstone 的 ID 不能再次写入。
- 设备 token、Admin session、Model Gateway 凭据是三类互相隔离的凭据。
- 所有敏感变更都必须写入审计日志。

## 9. 验收标准

1. Go Server 和 migration 二进制可以通过一条本地命令连接 PostgreSQL 启动。
2. 合法 Core 事件第一次接收为 accepted，重放返回 duplicate。
3. 客户端不能覆盖服务端推导出的租户、设备和主体身份。
4. 一个主体可以拥有多个终端，每条事件都带稳定的工作角色快照。
5. Admin Web 可以配置 `研发`、`测试`、`产品`，并按工作角色过滤事件。
6. 访问角色与工作角色不能混用；越权项目查询会被拒绝。
7. Model Gateway 可以使用 Ollama mock 和 Dify mock 运行，无需真实模型服务。
8. Admin Web 可以独立构建，Go Server 可以托管构建后的静态资源。
9. 部署会检查 `/opt/aetheris` 空间预算，且不要求源码保存服务器凭据。

