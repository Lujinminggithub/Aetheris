# Aetheris 服务端实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: 使用 `superpowers:subagent-driven-development` 或 `superpowers:executing-plans` 按任务逐项实现。每一步使用复选框跟踪。

**目标：** 将当前 Python Gateway 改造成可独立运行的 Go Server，并交付 PostgreSQL/RLS、React Admin Web、Python Ollama/Dify Model Gateway、部署入口和自动化测试。

**架构：** Go Server 是唯一的事件接收、鉴权、RBAC、查询和审计入口；Admin Web 只调用 API；Model Gateway 只接收 Go Server 筛选后的最小事件投影；Windows Core/Lens 在本计划中只作为兼容客户端，不修改其实现。

**技术栈：** Go 1.22+、`pgx/v5`、PostgreSQL 15+、React 18、TypeScript、Vite、Python 3.11+ 标准库 HTTP、Docker Compose、systemd。

**设计文档：** `docs/superpowers/specs/2026-09-06-aetheris-server-architecture-design.md`

## 全局约束

- 所有新增或修改文档使用中文；API 路径、表名、代码标识保留英文。
- 工作角色（`研发`/`测试`/`产品`）与 Admin 访问角色完全分离。
- 一台 Windows 终端只对应一个主体；一个主体可以对应多个终端。
- 服务端从设备凭证推导 `tenant_id`、`subject_id`、`device_id`，不信任客户端身份字段。
- 事件以 `event_id` 幂等，tombstone 后禁止重放。
- 日志不得包含密码、token、事件 payload、原始 prompt 或模型 response。
- PostgreSQL 是生产事实源；对象存储和向量索引只能通过独立接口接入。
- 不在源代码、脚本、文档和测试中保存服务器密码或明文部署凭据。

### 任务 1：服务端目录、契约与本地 PostgreSQL 基础

**文件：**
- 创建：`server/go.mod`、`server/cmd/aetheris-server/main.go`、`server/cmd/aetheris-migrate/main.go`
- 创建：`contracts/openapi.yaml`、`contracts/aetheris-event-v1.schema.json`
- 创建：`deploy/docker-compose.yml`、`deploy/.env.example`
- 创建：`server/internal/config/config.go`、`server/internal/httpx/json.go`
- 测试：`server/internal/config/config_test.go`

**接口：**
- 产出 `config.Load() (Config, error)`，从环境变量读取 `HTTP_ADDR`、`DATABASE_URL`、`ADMIN_SESSION_TTL`、`MODEL_GATEWAY_URL`、`MODEL_GATEWAY_TOKEN`。
- 产出 `httpx.WriteJSON(w, status, value)` 和 `httpx.ReadJSON(r, maxBytes, dst)`。
- 产出 OpenAPI 路径骨架，后续 handler 只能实现该契约中的路径。

- [ ] **步骤 1：写配置加载失败测试**

```go
func TestLoadRequiresDatabaseURL(t *testing.T) {
    t.Setenv("HTTP_ADDR", "127.0.0.1:8080")
    t.Setenv("DATABASE_URL", "")
    if _, err := Load(); err == nil {
        t.Fatal("expected DATABASE_URL error")
    }
}
```

- [ ] **步骤 2：运行测试确认失败**

运行：`go -C server test ./internal/config -run TestLoadRequiresDatabaseURL -v`

预期：因 `server/internal/config` 尚不存在而失败。

- [ ] **步骤 3：实现配置、JSON 辅助函数和命令入口**

`Config` 至少包含 `HTTPAddr`、`DatabaseURL`、`AdminSessionTTL`、`ModelGatewayURL`、`ModelGatewayToken`；缺少 `DATABASE_URL` 时返回明确错误。`docker-compose.yml` 只启动 PostgreSQL，不包含生产密码。

- [ ] **步骤 4：运行测试和编译**

运行：`go -C server test ./...`、`go -C server build ./cmd/aetheris-server`、`go -C server build ./cmd/aetheris-migrate`

预期：PASS，生成的二进制可以启动但在无数据库时返回配置/连接错误。

### 任务 2：PostgreSQL migration、租户模型和 RLS

**文件：**
- 创建：`server/migrations/001_identity.sql`、`server/migrations/002_roles.sql`、`server/migrations/003_events.sql`、`server/migrations/004_model_audit.sql`
- 创建：`server/internal/db/migrate.go`、`server/internal/db/db.go`
- 创建：`server/internal/db/testutil_test.go`
- 创建：`server/internal/db/migrate_test.go`
- 修改：`server/cmd/aetheris-migrate/main.go`

**接口：**
- 产出 `db.Open(ctx, url) (*pgxpool.Pool, error)`。
- 产出 `db.ApplyMigrations(ctx, pool, dir) error`，按文件名顺序记录 `schema_migrations`。
- 表必须包含 `tenants`、`users`、`access_roles`、`permissions`、`access_role_permissions`、`memberships`、`project_memberships`、`subjects`、`devices`、`device_credentials`、`work_roles`、`work_role_assignments`、`projects`、`events`、`event_tombstones`、`event_blobs`、`event_embeddings`、`model_runs`、`audit_logs`、`api_sessions`。

- [ ] **步骤 1：写 migration 结构测试**

```go
func openTestPoolFromEnv(t *testing.T) *pgxpool.Pool {
    t.Helper()
    url := os.Getenv("TEST_DATABASE_URL")
    if url == "" { t.Skip("TEST_DATABASE_URL is required") }
    pool, err := pgxpool.New(context.Background(), url)
    if err != nil { t.Fatal(err) }
    t.Cleanup(pool.Close)
    return pool
}
func testPostgres(t *testing.T) *pgxpool.Pool { t.Helper(); return openTestPoolFromEnv(t) }
func requireMigrations(t *testing.T, pool *pgxpool.Pool) { t.Helper(); if err := ApplyMigrations(context.Background(), pool, "../../migrations"); err != nil { t.Fatal(err) } }

func TestMigrationsCreateRequiredTables(t *testing.T) {
    pool := testPostgres(t)
    requireMigrations(t, pool)
    for _, name := range []string{"tenants", "subjects", "devices", "work_roles", "events", "audit_logs"} {
        var exists bool
        err := pool.QueryRow(context.Background(), `SELECT to_regclass($1) IS NOT NULL`, "public."+name).Scan(&exists)
        if err != nil || !exists { t.Fatalf("missing table %s: %v", name, err) }
    }
}
```

- [ ] **步骤 2：运行测试确认失败**

运行：`docker compose -f deploy/docker-compose.yml up -d postgres`，然后 `go -C server test ./internal/db -run TestMigrationsCreateRequiredTables -v`

预期：因 migration 和连接代码不存在而失败。

- [ ] **步骤 3：实现 migration 和 RLS**

为租户表添加 `tenant_id`；为 `events` 添加 `event_id` 主键、`payload JSONB`、角色快照列和唯一 `content_hash` 约束所需索引。启用 RLS，并定义使用事务变量 `aetheris.tenant_id` 的策略；Go repository 仍必须显式传入 tenant scope。

- [ ] **步骤 4：运行迁移测试**

运行：`go -C server test ./internal/db -v`。

预期：所有 migration 在空数据库执行一次成功，重复执行不改变结构且测试 PASS。

### 任务 3：事件验证、存储、幂等和审计 repository

**文件：**
- 创建：`server/internal/events/model.go`、`server/internal/events/validate.go`、`server/internal/events/repository.go`
- 创建：`server/internal/audit/repository.go`
- 创建：`server/internal/events/repository_test.go`

**接口：**
- `events.ParseAndValidate(raw []byte) (Event, error)`：校验 schema-v1 必填字段、RFC3339 时间、payload 对象和 content hash。
- `events.Repository.Insert(ctx, principal, event) (InsertStatus, error)`：返回 `accepted`、`duplicate` 或 `tombstoned`。
- `events.Repository.List(ctx, scope, filter) ([]Event, error)`：强制租户/项目 scope。
- `events.Repository.Tombstone(ctx, principal, eventID, reason) error`：事务内删除事件、写 tombstone、写 audit。
- `audit.Repository.Append(ctx, AuditRecord) error`：只追加，不记录敏感 payload。

- [ ] **步骤 1：写事件幂等和 hash 校验测试**

测试必须覆盖：首次插入 accepted、相同 ID duplicate、tombstone 后 tombstoned、修改 payload 后 hash mismatch、跨 tenant 查询为空。

- [ ] **步骤 2：运行失败测试**

运行：`go -C server test ./internal/events -v`。

预期：在 repository 未实现时失败。

- [ ] **步骤 3：实现 repository**

使用事务和 `INSERT ... ON CONFLICT`；插入前检查 tombstone；查询始终带 tenant 条件和可选 project 条件；审计记录只写 actor、action、resource、scope、result。

- [ ] **步骤 4：验证通过**

运行：`go -C server test ./internal/events ./internal/audit -v`。

预期：PASS，并检查日志测试中不存在 token、密码和 payload。

### 任务 4：认证、访问 RBAC、设备注册和工作角色服务

**文件：**
- 创建：`server/internal/auth/service.go`、`server/internal/auth/password.go`、`server/internal/auth/session.go`
- 创建：`server/internal/authorization/authorizer.go`
- 创建：`server/internal/devices/service.go`
- 创建：`server/internal/workroles/service.go`
- 创建：`server/internal/auth/service_test.go`、`server/internal/workroles/service_test.go`

**接口：**
- `auth.Service.Login(ctx, username, password) (Session, error)`。
- `auth.Service.Refresh(ctx, refreshToken) (Session, error)`。
- `authorization.Authorizer.Require(ctx, principal, permission, scope) error`。
- `devices.Service.Bootstrap(ctx, BootstrapRequest) (DeviceCredential, error)`。
- `devices.Service.ResolveCredential(ctx, token) (DevicePrincipal, error)`。
- `workroles.Service.Resolve(ctx, tenantID, subjectID, deviceID, projectID, at) (RoleSnapshot, error)`。
- `workroles.Service.Assign(ctx, actor, Assignment) error`。

- [ ] **步骤 1：写权限和角色优先级测试**

测试覆盖：`device_ingest` 只能 ingest；`analyst` 不能管理用户；项目覆盖优先于主体角色；角色无效或过期时回退租户默认；一个主体可绑定多个设备。

- [ ] **步骤 2：运行失败测试**

运行：`go -C server test ./internal/auth ./internal/authorization ./internal/workroles -v`。

预期：接口未实现时失败。

- [ ] **步骤 3：实现服务**

密码使用 Argon2id；session/设备 token 只存 SHA-256 哈希；生成 token 后只在响应中返回一次。所有写操作追加 audit。设备 bootstrap 使用一次性 enrollment secret，并创建 subject/device 绑定。

- [ ] **步骤 4：验证安全行为**

运行同上测试，并增加 `go -C server test ./... -run Sensitive -v`，确认错误和日志不包含明文凭据。

### 任务 5：Go Server HTTP API 与 Admin 静态托管

**文件：**
- 创建：`server/internal/httpapi/router.go`、`server/internal/httpapi/middleware.go`
- 创建：`server/internal/httpapi/device_handlers.go`、`server/internal/httpapi/ingest_handlers.go`
- 创建：`server/internal/httpapi/auth_handlers.go`、`server/internal/httpapi/admin_handlers.go`
- 创建：`server/internal/httpapi/static.go`
- 创建：`server/internal/httpapi/httpapi_test.go`
- 修改：`server/cmd/aetheris-server/main.go`

**接口：**
- 实现 spec 中所有 `/api/v1/device/*`、`/api/v1/ingest`、`/api/v1/auth/*`、`/api/v1/admin/*` endpoint。
- `POST /api/v1/ingest` 接收 `{events: [...]}`，限制 body 10 MiB、单批 1000 条，逐条返回状态。
- `GET /admin/*` 只托管 Admin Web 构建目录；不存在资源返回 index fallback 或 404，不读取任意路径。

- [ ] **步骤 1：写 HTTP 集成测试**

测试覆盖：health 公开可读；错误 token 为 401；首次事件 accepted、重放 duplicate；客户端伪造 tenant/device 被覆盖或拒绝；越权事件列表为 403/空结果；tombstone 需要权限。

- [ ] **步骤 2：运行失败测试**

运行：`go -C server test ./internal/httpapi -v`。

预期：路由和 handler 未实现时失败。

- [ ] **步骤 3：实现中间件和 handlers**

中间件顺序固定为 request-id -> recovery -> size limit -> auth -> CSRF（cookie mutation）-> handler。设备 API 使用 Bearer device token；Admin API 使用 HttpOnly session cookie。统一错误 JSON 为 `{error, message, request_id}`，不返回 stack trace。

- [ ] **步骤 4：运行 API 测试和编译**

运行：`go -C server test ./... -v`、`go -C server build ./cmd/aetheris-server`。

### 任务 6：React Admin Web

**文件：**
- 创建：`admin-web/package.json`、`admin-web/tsconfig.json`、`admin-web/vite.config.ts`、`admin-web/index.html`
- 创建：`admin-web/src/main.tsx`、`admin-web/src/api/client.ts`、`admin-web/src/api/types.ts`
- 创建：`admin-web/src/pages/LoginPage.tsx`、`admin-web/src/pages/DashboardPage.tsx`、`admin-web/src/pages/WorkRolesPage.tsx`、`admin-web/src/pages/EventsPage.tsx`、`admin-web/src/pages/AuditPage.tsx`
- 创建：`admin-web/src/styles.css`、`admin-web/src/App.test.tsx`

**接口：**
- `api/client.ts` 提供 `login`、`me`、`listDevices`、`listSubjects`、`listWorkRoles`、`assignWorkRole`、`listEvents`、`tombstoneEvent`、`listAuditLogs`、`createModelRun`。
- 所有 API 类型从 `contracts/openapi.yaml` 生成或与其保持严格一致。

- [ ] **步骤 1：写 API client 和页面状态测试**

测试必须覆盖：401 跳转登录、加载中状态、空列表状态、角色筛选参数、tombstone 成功/失败提示。

- [ ] **步骤 2：运行前端失败测试**

运行：`npm --prefix admin-web test -- --run`

预期：项目尚未创建时失败。

- [ ] **步骤 3：实现前端页面**

实现登录、设备/主体列表、工作角色分配、事件按工作角色过滤、审计列表和模型运行状态；不在前端复制权限判断。CSS 保持紧凑、可扫描，错误和空状态可见。

- [ ] **步骤 4：构建并验证静态产物**

运行：`npm --prefix admin-web run build`。

预期：生成 `admin-web/dist`，可由 Go Server 静态 handler 托管。

### 任务 7：Python Model Gateway 与 provider 适配器

**文件：**
- 创建：`model-gateway/pyproject.toml`、`model-gateway/aetheris_model_gateway/__init__.py`
- 创建：`model-gateway/aetheris_model_gateway/config.py`、`model-gateway/aetheris_model_gateway/protocols.py`
- 创建：`model-gateway/aetheris_model_gateway/providers/ollama.py`、`model-gateway/aetheris_model_gateway/providers/dify.py`
- 创建：`model-gateway/aetheris_model_gateway/http.py`
- 创建：`model-gateway/tests/test_providers.py`、`model-gateway/tests/test_http.py`

**接口：**
- `ModelProvider.generate(request: ModelRequest) -> ModelResponse`。
- `OllamaProvider` 调用 `${OLLAMA_URL}/api/chat`。
- `DifyProvider` 调用配置的 workflow/app endpoint，API key 只从环境变量读取。
- `POST /internal/v1/generate` 使用独立 `MODEL_GATEWAY_TOKEN`，限制请求大小、超时和响应大小。

- [ ] **步骤 1：写 mock provider 测试**

测试覆盖：provider 成功、超时、非 2xx、无效 JSON、超长 response、缺少凭据；测试不得访问真实 Ollama/Dify。

- [ ] **步骤 2：运行失败测试**

运行：`python -m unittest discover -s model-gateway/tests -v`

预期：模块未实现时失败。

- [ ] **步骤 3：实现协议、provider 和 HTTP 服务**

使用 Python 标准库 `urllib`/`http.server`；重试只针对明确的连接错误和 5xx，最多 2 次；响应统一为 `run_id/status/provider/model/result/error`，日志只记录 run_id、状态、耗时和错误码。

- [ ] **步骤 4：运行模型服务测试**

运行：`python -m unittest discover -s model-gateway/tests -v`。

### 任务 8：对象存储、向量索引和容量健康检查接口

**文件：**
- 创建：`server/internal/storage/blob.go`、`server/internal/storage/vector.go`
- 创建：`server/internal/health/checks.go`、`server/internal/health/checks_test.go`

**接口：**
- `BlobStore.Put(ctx, ref, reader) error`、`BlobStore.Delete(ctx, ref) error`、`BlobStore.Health(ctx) error`。
- `VectorIndex.Upsert(ctx, embedding) error`、`VectorIndex.Search(ctx, query) ([]Hit, error)`、`VectorIndex.Health(ctx) error`。
- provider 未配置时返回 `disabled`，不得阻塞事件 ingest。

- [ ] **步骤 1：写 disabled provider 测试**

验证未配置对象存储或向量索引时 health 返回 disabled，事件入库仍成功。

- [ ] **步骤 2：实现接口和启动检查**

先提供 no-op provider 和接口契约；启动检查 PostgreSQL、磁盘预算、可选 provider 状态，80% 告警，90% 拒绝非必要导入。

- [ ] **步骤 3：运行测试**

运行：`go -C server test ./internal/storage ./internal/health -v`。

### 任务 9：部署资产、备份和中文运维文档

**文件：**
- 创建：`deploy/systemd/aetheris-server.service`、`deploy/systemd/aetheris-model-gateway.service`
- 创建：`deploy/install-server.sh`、`deploy/backup-postgres.sh`
- 创建：`docs/operations/server-deployment.md`、`docs/operations/server-operations.md`
- 修改：`README.md`

**接口：**
- 安装脚本只接受环境变量 `AETHERIS_RELEASE_DIR`、`AETHERIS_CONFIG_FILE`，不接受或保存 root 密码。
- systemd 服务使用非 root 用户、`NoNewPrivileges=true`、独立 WorkingDirectory。
- README 提供本地启动、迁移、构建 Admin、启动 Model Gateway、运行测试的中文命令。

- [ ] **步骤 1：写部署检查脚本测试**

使用临时目录验证目录权限、缺少配置时失败、不会把敏感环境变量写入日志。

- [ ] **步骤 2：实现 systemd/备份/安装脚本**

部署前只读检查磁盘和现有目录；备份使用 `pg_dump` 输出到 `backups/`，按保留天数轮转；不覆盖未知文件。

- [ ] **步骤 3：验证部署资产**

运行：`docker compose -f deploy/docker-compose.yml config`、`bash -n deploy/install-server.sh`、`bash -n deploy/backup-postgres.sh`。

### 任务 10：端到端验收与现有回归

**文件：**
- 创建：`server/tests/e2e_test.go`、`scripts/run_server_checks.ps1`
- 修改：`tests/test_gateway.py`（仅在需要增加兼容断言时）

**接口：**
- 端到端流程：创建 tenant/subject/device -> bootstrap -> ingest -> duplicate -> Admin 登录 -> 配置工作角色 -> 按角色查询事件 -> mock model run -> audit 查询。

- [ ] **步骤 1：启动依赖并运行端到端测试**

运行：`docker compose -f deploy/docker-compose.yml up -d postgres`，`go -C server test ./tests -v`。

- [ ] **步骤 2：运行前端和 Model Gateway 测试**

运行：`npm --prefix admin-web test -- --run`、`python -m unittest discover -s model-gateway/tests -v`。

- [ ] **步骤 3：运行现有 Python 回归测试**

运行：`python -m unittest discover -s tests -v`、`python -m compileall src tests model-gateway`。

- [ ] **步骤 4：构建所有服务端产物**

运行：`go -C server build ./...`、`npm --prefix admin-web run build`、`python -m compileall model-gateway`。

- [ ] **步骤 5：完成验收记录**

将命令、版本、通过/失败结果和已知限制写入中文 `docs/operations/implementation-log.md`，不写入任何 token、密码或原始事件内容。
