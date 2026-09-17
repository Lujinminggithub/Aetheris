# 过程知识增强智能查询实施计划

> **执行说明：** 按任务顺序实施并使用复选框（`- [ ]`）跟踪状态；执行时遵循既有测试驱动和验收流程。

**目标：** 将 `safe` 等逻辑项目中的完整人机协作过程提炼为可追溯过程知识，使用混合检索和通用模型补全生成答案，并在部署时自动准备 Ollama 生成与向量模型。

**架构：** PostgreSQL 保存版本化会话、轮次、知识、证据、分块和任务；Qdrant 使用独立 `aetheris_process_knowledge_v1` 集合保存知识向量；Go Server 负责项目归属、知识提取、混合检索、权限和答案校验；Python Model Gateway 负责受约束的计划与回答生成；Admin Web 提供项目限定查询和知识诊断。

**技术栈：** Go 1.22、PostgreSQL 14/pg_trgm、Qdrant 1.19.1、Python 3.11、Ollama、React/TypeScript/Vite、systemd、Bash。

**规格文档：** `docs/superpowers/specs/2026-09-16-process-knowledge-rag-design.md`

## 全局约束

- `events` 与 `clean_event_facts` 不可变，所有知识均通过 revision 和版本重算。
- 项目范围统一使用 `logical_project_id`；未归属工具兜底事件不能进入已确认项目知识索引。
- 用户问题只表示意图，不能单独支持技术事实。
- 本地已验证结果优先于通用知识，并必须返回适用条件与证据。
- 默认答案不能输出当前项目能力清单。
- Qdrant 不保存正文；AI 命令仍不保存完整命令。
- 新旧 RAG 并存，过程知识查询支持 shadow、canary、active 和回滚。
- 远期训练只导出 `accepted + verified + active` 知识，本轮不修改模型权重。

---

### 任务 1：Ollama 模型自动准备

**文件：**
- 新增： `deploy/pull-models.sh`
- 修改： `deploy/install-server.sh`
- 修改： `deploy/systemd/ollama.service`
- 修改： `docs/operations/server-deployment.md`
- 测试： `tests/test_model_provisioning.py`

**接口：**
- 输入： `OLLAMA_HOST`、`OLLAMA_GENERATION_MODEL`、`OLLAMA_EMBEDDING_MODEL`。
- 输出： 幂等命令 `deploy/pull-models.sh`；systemd 启动后自动拉取并预热 `qwen3:1.7b`，同时准备 `embeddinggemma`。`qwen3:4b-instruct` 可作为高质量模型保留，但不作为纯 CPU 在线默认值。

- [x] 编写失败测试，断言脚本具有就绪重试、模型去重、`ollama pull` 和固定用户目录，unit 包含 `ExecStartPost`。
- [x] 运行 `python -m unittest tests.test_model_provisioning -v`，确认因脚本缺失失败。
- [x] 实现只输出模型名和状态、不输出环境凭据的幂等脚本，并让安装脚本复制到 `/opt/aetheris/bin`。
- [x] 更新 unit 与中文部署文档。
- [x] 重跑测试并执行 `bash -n deploy/pull-models.sh deploy/install-server.sh`。

### 任务 2：过程知识数据库与权限

**文件：**
- 新增： `server/migrations/020_process_knowledge.sql`
- 新增： `server/internal/processknowledge/migration_test.go`
- 修改： `server/internal/cleaning/migration_test.go`

**接口：**
- 输出： `process_sessions`、`process_turns`、`process_knowledge_units`、`process_knowledge_evidence`、`process_knowledge_chunks`、`process_knowledge_jobs`、`process_knowledge_state`；权限 `process_knowledge:read/manage`。

- [x] 编写 migration 失败测试，检查约束、RLS、GIN/trigram/范围索引和权限。
- [x] 运行 `go test ./internal/processknowledge ./internal/cleaning -run Migration -v`，确认缺少 migration。
- [x] 实现 migration，状态枚举与规格完全一致，并为 `pg_trgm` 建立关键词索引。
- [x] 重跑测试。

### 任务 3：Supersession 逻辑项目继承

**文件：**
- 修改： `server/internal/projectattribution/repository.go`
- 修改： `server/internal/projectattribution/rules.go`
- 修改： `server/internal/projectattribution/rules_test.go`
- 修改： `server/internal/projectattribution/repository_test.go`

**接口：**
- 输出： `ResolveSupersededProject(replacementEventID)`，替代事件在没有更强证据时继承旧事件当前逻辑项目。

- [x] 新增失败测试：原事件属于 `safe`，替代事件原始项目为 `Codex`，结果仍为 `safe` 且原因码为 `superseded_event_inheritance`。
- [x] 运行项目归属测试确认失败。
- [x] 在回填候选加载和规则优先级中加入继承证据；显式精确绑定仍可覆盖继承。
- [x] 重跑 `go test ./internal/projectattribution -v`。

### 任务 4：会话重建与轮次分类

**文件：**
- 新增： `server/internal/processknowledge/model.go`
- 新增： `server/internal/processknowledge/session.go`
- 新增： `server/internal/processknowledge/session_test.go`
- 新增： `server/internal/processknowledge/classify.go`
- 新增： `server/internal/processknowledge/classify_test.go`

**接口：**
- 输出： `AssembleSessions([]SourceTurn) []SessionDraft`、`ClassifyTurn(SourceTurn) StatementKind`。
- 精确键：tenant、device、logical project、AI tool、session ID；缺少 ID 才允许低置信度时间推断。

- [x] 编写失败测试，覆盖相同时间不同 session 不串联、人工问题/确认/否定、AI 探索/最终结论和工具验证。
- [x] 运行测试确认类型和函数不存在。
- [x] 实现确定性会话组装与轮次分类；长 AI 回复和结构化完成标记优先识别为最终回答。
- [x] 重跑测试。

### 任务 5：知识提取、分块与证据状态

**文件：**
- 新增： `server/internal/processknowledge/extract.go`
- 新增： `server/internal/processknowledge/extract_test.go`
- 新增： `server/internal/processknowledge/chunk.go`
- 新增： `server/internal/processknowledge/chunk_test.go`

**接口：**
- 输出： `ExtractKnowledge(SessionDraft) []KnowledgeDraft`、`ChunkKnowledge(KnowledgeDraft, min, max int) []ChunkDraft`。
- 用户问题成为 `problem/intent`；只有 AI 最终回答产生 `conclusion`；确认更新决策状态；验证结果更新验证状态。

- [x] 编写失败测试：单独用户问题不生成结论；55,000 字后半段 EDR 内容可形成块；API 名称不在中间截断。
- [x] 运行测试确认失败。
- [x] 实现最小确定性提取器、证据链和 600-1,000 字语义分块。
- [x] 重跑测试。

### 任务 6：Repository、任务与增量 Worker

**文件：**
- 新增： `server/internal/processknowledge/repository.go`
- 新增： `server/internal/processknowledge/repository_test.go`
- 新增： `server/internal/processknowledge/service.go`
- 新增： `server/internal/processknowledge/service_test.go`
- 新增： `server/internal/processknowledge/worker.go`
- 新增： `server/internal/processknowledge/worker_test.go`
- 修改： `server/cmd/aetheris-server/main.go`
- 修改： `server/internal/config/config.go`
- 修改： `server/internal/config/config_test.go`

**接口：**
- 输出： `SyncSessions`、`ExtractDirtySessions`、`StartBackfill`、`ActivateVersion`、`RollbackVersion`、`Summary`、`ListUnits`、`GetUnit`。
- Worker 使用持久游标和有界批次，失败不阻塞 ingest。

- [x] 编写 repository/service/worker 失败测试，覆盖幂等、dirty revision、dry-run/apply/resume/activate/rollback。
- [x] 运行测试确认失败。
- [x] 实现 PostgreSQL repository 与 Worker，并增加 `PROCESS_KNOWLEDGE_*` 配置。
- [x] 重跑包测试和 config 测试。

### 任务 7：独立知识向量与混合检索

**文件：**
- 新增： `server/internal/processknowledge/search.go`
- 新增： `server/internal/processknowledge/search_test.go`
- 新增： `server/internal/processknowledge/indexer.go`
- 新增： `server/internal/processknowledge/indexer_test.go`
- 修改： `server/internal/retrieval/query_service.go`
- 修改： `server/internal/retrieval/model.go`
- 修改： `server/internal/retrieval/query_test.go`
- 修改： `server/internal/config/config.go`
- 修改： `server/cmd/aetheris-server/main.go`

**接口：**
- 输出： `KnowledgeSearcher.Search(ctx, KnowledgeQuery) ([]KnowledgeHit, error)`。
- 双路召回使用 Qdrant 稠密向量与 PostgreSQL trigram；Go 使用 RRF、知识单元去重、会话配额和验证状态优先。

- [x] 编写失败测试，覆盖精确 `WFP` 命中、语义命中、RRF、项目隔离、同会话去重和 verified 优先。
- [x] 运行测试确认失败。
- [x] 实现 `aetheris_process_knowledge_v1` 索引器和搜索器。
- [x] 将 `knowledge_scope=project_process` 查询接入 QueryService，旧活动 RAG 继续兼容。
- [x] 重跑 processknowledge/retrieval 测试。

### 任务 8：两阶段回答与动态生成预算

**文件：**
- 修改： `server/internal/retrieval/generator.go`
- 修改： `server/internal/retrieval/generator_test.go`
- 修改： `server/internal/retrieval/answer.go`
- 修改： `server/internal/retrieval/answer_test.go`
- 修改： `model-gateway/aetheris_model_gateway/protocols.py`
- 修改： `model-gateway/aetheris_model_gateway/providers/ollama.py`
- 修改： `model-gateway/tests/test_providers.py`

**接口：**
- 输出： Go Server 先生成确定性 `answer_plan`，Model Gateway 再执行一次 `rag_answer`；纯 CPU 在线预算为 analysis 384 token、reason/procedure/numeric 256 token、direct 128 token。`plan_rag_answer` 仅保留兼容，不再用于在线主链路。
- 本地引用带 `knowledge_id/source_kind/validation_state/applicability`。

- [x] 编写 Go/Python 失败测试，覆盖先计划后回答、动态 token、用户问题不能作为事实、验证冲突优先。
- [x] 运行测试确认失败。
- [x] 实现计划结构、模型调用和服务端校验；生成失败使用已验证知识确定性保底。
- [x] 重跑 Go retrieval 与 Model Gateway 测试。

### 任务 9：Admin API 与 OpenAPI

**文件：**
- 新增： `server/internal/httpapi/process_knowledge_handlers.go`
- 新增： `server/internal/httpapi/process_knowledge_handlers_test.go`
- 修改： `server/internal/httpapi/router.go`
- 修改： `server/internal/httpapi/rag_handlers.go`
- 修改： `contracts/openapi.yaml`
- 修改： `server/cmd/aetheris-server/main.go`

**接口：**
- 输出： `/admin/process-knowledge/summary|units|jobs|versions`；RAG 请求支持 `logical_project_id`、`knowledge_scope`、显式全部项目。

- [x] 编写失败的 handler 与契约测试，覆盖权限、分页、项目必选、任务状态和引用字段。
- [x] 运行测试确认失败。
- [x] 实现 handler、路由、权限和 OpenAPI。
- [x] 重跑 httpapi 测试并解析 OpenAPI YAML。

### 任务 10：Admin Web 过程知识与查询体验

**文件：**
- 新增： `admin-web/src/pages/ProcessKnowledgePage.tsx`
- 新增： `admin-web/src/pages/ProcessKnowledgePage.test.tsx`
- 修改： `admin-web/src/pages/RAGQueryPage.tsx`
- 修改： `admin-web/src/pages/RAGQueryPage.test.tsx`
- 修改： `admin-web/src/api/types.ts`
- 修改： `admin-web/src/api/client.ts`
- 修改： `admin-web/src/App.tsx`
- 修改： `admin-web/src/styles.css`

**接口：**
- 查询默认选择逻辑项目；“全部项目”必须显式选择。
- 页面展示知识状态、来源构成、覆盖主题、适用条件和证据链，不展示项目能力清单。

- [x] 编写失败组件测试，覆盖项目必选、显式全部项目、验证标签、引用展开、回填和空/错/加载状态。
- [x] 运行 Vitest 确认失败。
- [x] 实现类型、API、页面、导航和样式。
- [x] 重跑前端测试、类型检查和构建。

### 任务 11：回填 CLI、评测数据集与生产验收

**文件：**
- 修改： `server/cmd/aetheris-admin/main.go`
- 修改： `server/cmd/aetheris-admin/main_test.go`
- 新增： `server/internal/processknowledge/evaluation.go`
- 新增： `server/internal/processknowledge/evaluation_test.go`
- 新增： `docs/operations/process-knowledge-rag-acceptance.md`
- 修改： `docs/operations/server-deployment.md`
- 修改： `docs/operations/server-operations.md`
- 修改： `docs/operations/implementation-log.md`

**接口：**
- 输出： `aetheris-admin process-knowledge-backfill`、`process-knowledge-export-eval`；训练候选仅导出 accepted+verified+active。

- [x] 编写失败测试，固定 EDR 问题必须覆盖架构主题、引用过程知识且不输出能力清单。
- [x] 运行测试确认失败。
- [x] 实现 CLI、评测导出和运维文档。
- [x] 运行 Python、Go、Model Gateway、Admin Web、VS Code 全量测试与构建。
- [x] 生产已执行 migration、dry-run、shadow 对账、版本 8 canary 10%、回滚到版本 7及恢复版本 8 shadow 演练。
