# Aetheris 统一活动与本地 RAG 实施计划

> **面向执行代理：** 必须使用 `superpowers:subagent-driven-development`（推荐）或 `superpowers:executing-plans`，按任务执行并使用复选框跟踪状态。

**目标：** 交付统一活动记录、AI 工具兜底项目、Qdrant/Ollama 本地 RAG 和带证据引用的异步智能查询入口。

**架构：** Core 只负责本地项目归因和脱敏；Go Cleaning 将全部可查询事件物化为统一事实；Retrieval Indexer 通过 Model Gateway 获取 embedding 并写 Qdrant；Go 异步查询 worker 负责权限过滤、检索、生成和引用，Admin Web 只消费分页/任务 API。

**技术栈：** Python 3.12/PyInstaller、Go 1.22/pgx、PostgreSQL 14、Qdrant 1.19.x、Ollama `embeddinggemma` 与 `qwen3:4b-instruct`、React/TypeScript/Vite、systemd。

**设计文档：** `docs/superpowers/specs/2026-09-07-unified-activity-rag-design.md`

## 全局约束

- 原始 `events` 不更新、不覆盖、不删除。
- fallback AI 项目不得上传 cwd 或本机完整路径。
- AI 工具只保存和索引工具名、命令类型、安全摘要、HMAC；禁止完整命令和 arguments/input。
- RAG 只索引当前规则版本中 accepted/merged 且未排除的清洗事实。
- Qdrant 只监听 loopback，正文只保存在 PostgreSQL。
- 所有查询必须经过 session、RBAC、tenant 和项目范围复核。
- 不增加员工排名、评分或绩效判断。
- 当前目录没有 `.git`，不执行提交步骤；以测试、备份和部署验收作为关口。
- 所有新增和修改文档使用中文。

---

### 任务 1：Core AI 逻辑项目归因

**文件：**
- 创建：`src/aetheris/project_attribution.py`
- 修改：`src/aetheris/tray.py`、`src/aetheris/version.py`、`pyproject.toml`
- 测试：`tests/test_project_attribution.py`、`tests/test_tray.py`

**接口：**
- 产出：`attribute_ai_project(tool, cwd, authorized_roots) -> AIProjectAttribution`
- `AIProjectAttribution(project_id, project_label, attribution, project_root)`

- [x] 编写失败测试，证明授权 cwd 映射仓库末级名称、未授权 cwd 映射 `Codex`、fallback payload 不含 cwd，并且非 AI 越权记录仍被拒绝。
- [x] 运行定向测试，确认失败由 fallback 归因缺失导致。
- [x] 实现确定性逻辑项目 ID，在事件序列化前清理 AI payload，并使用 supersession 保持历史可追溯和统计唯一。
- [x] 将 Core/Setup 升级到 0.4.5，运行定向和全量 Core 测试。

### 任务 2：统一活动事实与 API

**文件：**
- 创建：`server/migrations/008_unified_activity_rag.sql`
- 创建：`server/internal/activities/model.go`、`repository.go`、`repository_test.go`
- 修改：`server/internal/cleaning/model.go`、`rules.go`、`repository.go`、`rules_test.go`
- 修改：`server/internal/httpapi/router.go`、`server/internal/httpapi/activity_handlers.go`
- 修改：`server/cmd/aetheris-server/main.go`、`contracts/openapi.yaml`

**接口：**
- 产出：`activities.Repository.Query(context.Context, activities.Filter) (activities.Result, error)`
- 端点：`GET /api/v1/admin/activities`

- [x] 编写 Git/IDE/浏览器一对一事实及待确认事实排除的失败测试。
- [x] 编写活动分类、项目标签安全、分页和 AI 二级角色的失败测试。
- [x] 运行 Go 定向测试，确认事实和 API 行为缺失。
- [x] 为事实增加 `event_type`、`source`、`activity_type`，实现当前规则版本的分页统一查询和维度统计。
- [x] 保持 `/admin/ai-interactions` 兼容，并增加 OpenAPI 合同。
- [x] 运行受影响的 Go 测试、`go vet ./...` 和 Linux 交叉构建。

### 任务 3：统一 Admin 活动页面

**文件：**
- 创建：`admin-web/src/pages/ActivitiesPage.tsx`、`ActivitiesPage.test.tsx`
- 修改：`admin-web/src/App.tsx`、`App.test.tsx`、`src/api/types.ts`、`src/api/client.ts`、`src/styles.css`
- 从导航移除：仅移除 `AIInteractionsPage`、`EventsPage` 路由；源码暂时保留用于兼容测试。

**接口：**
- 使用：`GET /api/v1/admin/activities`

- [x] 编写组件失败测试，证明只有一个“活动记录”导航，不存在独立 AI/事件入口，并覆盖服务端分页及各活动筛选。
- [x] 运行定向 Vitest，确认旧导航导致失败。
- [x] 实现工作型活动页面、事实/证据详情抽屉和加载/错误/空状态。
- [x] 运行完整 Vitest 和 Vite 生产构建。

### 任务 4：Model Gateway embedding 端点

**文件：**
- 修改：`model-gateway/aetheris_model_gateway/config.py`、`protocols.py`、`http.py`
- 创建：`model-gateway/aetheris_model_gateway/providers/ollama_embeddings.py`
- 测试：`model-gateway/tests/test_embeddings.py`、`test_http.py`、`test_config.py`

**接口：**
- 端点：`POST /internal/v1/embed`
- 请求：`{tenant_id, model, inputs[]}`
- 响应：`{model, dimensions, embeddings[][]}`

- [x] 编写认证、批量上限、异常向量、provider 超时和就绪字段的失败测试。
- [x] 运行 Model Gateway 测试，确认 `/internal/v1/embed` 缺失。
- [x] 实现有界 Ollama `/api/embed` 适配器和独立 embedding 模型配置。
- [x] 运行全部 Model Gateway 测试和 `compileall`。

### 任务 5：Qdrant 索引与检索文档

**文件：**
- 创建：`server/internal/retrieval/model.go`、`document.go`、`qdrant.go`、`embedding.go`、`repository.go`、`indexer.go`
- 测试：`server/internal/retrieval/document_test.go`、`qdrant_test.go`、`indexer_test.go`
- 修改：`server/internal/config/config.go`、`server/cmd/aetheris-server/main.go`、`deploy/.env.example`

**接口：**
- `EmbeddingClient.Embed(ctx, []string) ([][]float32, error)`
- `VectorIndex.Upsert(ctx, []Point) error`, `Query(ctx, vector, Filter, limit) ([]Hit, error)`, `Delete(ctx, []string) error`
- `Indexer.RunOnce(ctx, tenantID, limit) (IndexResult, error)`

- [x] 编写失败测试，确保 AI 工具不暴露命令、排除事实不生成文档、ID 确定且 Qdrant 请求含 tenant 过滤。
- [x] 运行 retrieval 测试，确认实现缺失。
- [x] 实现安全文档投影、PostgreSQL 状态流转、有界 embedding 批次和 Qdrant REST 适配器。
- [x] 接入不阻塞 ingest 的周期索引器，并运行 Go 测试、vet 和构建。

### 任务 6：异步 RAG 查询 API 与 Admin 页面

**文件：**
- 创建：`server/internal/retrieval/query_service.go`、`query_test.go`、`gate.go`
- 创建：`server/internal/httpapi/rag_handlers.go`
- 修改：`server/internal/httpapi/router.go`、`server/cmd/aetheris-server/main.go`、`contracts/openapi.yaml`
- 创建：`admin-web/src/pages/RAGQueryPage.tsx`、`RAGQueryPage.test.tsx`
- 修改：`admin-web/src/App.tsx`、`src/api/types.ts`、`src/api/client.ts`、`src/styles.css`

**接口：**
- `POST /api/v1/admin/rag/queries`
- `GET /api/v1/admin/rag/queries/{id}`
- `GET /api/v1/admin/rag/status`

- [x] 编写 202 创建、tenant 范围、阶段流转、引用白名单和 provider 失败状态的 Go 失败测试。
- [x] 编写范围筛选、进度显示、失败状态和可点击引用的 React 失败测试。
- [x] 实现异步查询任务、Qdrant 检索、PostgreSQL 授权复核、Ollama 生成和结构化引用。
- [x] 实现“智能查询”页面、轮询清理和查询优先并发门。
- [x] 运行全部 Go/Admin 测试、vet 和生产构建。

### 任务 7：基础设施、回填与生产验收

**文件：**
- 创建：`deploy/systemd/qdrant.service`、`deploy/qdrant/config.yaml`
- 修改：`deploy/systemd/ollama.service`、`deploy/.env.example`
- 修改：`docs/operations/server-deployment.md`、`docs/operations/server-implementation-log.md`
- 产物：`releases/0.4.5/AetherisCore-0.4.5.exe`、`AetherisSetup-0.4.5.exe`

- [x] 运行 Core、Go、Model Gateway、Admin Web 和原生 provisioning 测试套件。
- [x] 构建并校验 Core/Setup、Linux Go 二进制和 Admin 静态资源。
- [x] 备份 PostgreSQL、服务、配置、Core 队列/checkpoint 和当前产物。
- [x] 安装经官方哈希验证的 Qdrant x86_64 发行版，作为 loopback systemd 服务并使用 `/opt/aetheris/vector/qdrant` 存储。
- [x] 对 BGE-M3 与 EmbeddingGemma 做同批生产基准，采用更快的 `embeddinggemma`，验证 `/api/embed`，并按依赖顺序部署 migration、Model Gateway、Go、Admin 和 Core。
- [x] 重放 AI checkpoint，验证 fallback 项目不含 cwd，既有事件按 duplicate/supersession 处理。
- [x] 重算统一事实，分批索引可索引锚点事实，并对账 PostgreSQL/Qdrant 数量。
- [x] 验证异步查询返回可打开引用、AI 命令泄漏为零，并确认所有服务正常。
- [x] 更新中文运维记录，仅清理已验证可再生成的 staging 文件。
