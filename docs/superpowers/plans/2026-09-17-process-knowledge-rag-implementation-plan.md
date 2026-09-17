# 过程知识增强智能查询实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 `safe` 等逻辑项目中的完整人机协作过程提炼为可追溯过程知识，使用混合检索和通用模型补全生成答案，并在部署时自动准备 Ollama 生成与向量模型。

**Architecture:** PostgreSQL 保存版本化会话、轮次、知识、证据、分块和任务；Qdrant 使用独立 `aetheris_process_knowledge_v1` 集合保存知识向量；Go Server 负责项目归属、知识提取、混合检索、权限和答案校验；Python Model Gateway 负责受约束的计划与回答生成；Admin Web 提供项目限定查询和知识诊断。

**Tech Stack:** Go 1.22、PostgreSQL 14/pg_trgm、Qdrant 1.19.1、Python 3.11、Ollama、React/TypeScript/Vite、systemd、Bash。

**Spec:** `docs/superpowers/specs/2026-09-16-process-knowledge-rag-design.md`

## Global Constraints

- `events` 与 `clean_event_facts` 不可变，所有知识均通过 revision 和版本重算。
- 项目范围统一使用 `logical_project_id`；未归属工具兜底事件不能进入已确认项目知识索引。
- 用户问题只表示意图，不能单独支持技术事实。
- 本地已验证结果优先于通用知识，并必须返回适用条件与证据。
- 默认答案不能输出当前项目能力清单。
- Qdrant 不保存正文；AI 命令仍不保存完整命令。
- 新旧 RAG 并存，过程知识查询支持 shadow、canary、active 和回滚。
- 远期训练只导出 `accepted + verified + active` 知识，本轮不修改模型权重。

---

### Task 1: Ollama 模型自动准备

**Files:**
- Create: `deploy/pull-models.sh`
- Modify: `deploy/install-server.sh`
- Modify: `deploy/systemd/ollama.service`
- Modify: `docs/operations/server-deployment.md`
- Test: `tests/test_model_provisioning.py`

**Interfaces:**
- Consumes: `OLLAMA_HOST`、`OLLAMA_GENERATION_MODEL`、`OLLAMA_EMBEDDING_MODEL`。
- Produces: 幂等命令 `deploy/pull-models.sh`；systemd 启动后自动拉取 `qwen3:4b-instruct` 和 `embeddinggemma`。

- [x] 编写失败测试，断言脚本具有就绪重试、模型去重、`ollama pull` 和固定用户目录，unit 包含 `ExecStartPost`。
- [x] 运行 `python -m unittest tests.test_model_provisioning -v`，确认因脚本缺失失败。
- [x] 实现只输出模型名和状态、不输出环境凭据的幂等脚本，并让安装脚本复制到 `/opt/aetheris/bin`。
- [x] 更新 unit 与中文部署文档。
- [x] 重跑测试并执行 `bash -n deploy/pull-models.sh deploy/install-server.sh`。

### Task 2: 过程知识数据库与权限

**Files:**
- Create: `server/migrations/020_process_knowledge.sql`
- Create: `server/internal/processknowledge/migration_test.go`
- Modify: `server/internal/cleaning/migration_test.go`

**Interfaces:**
- Produces: `process_sessions`、`process_turns`、`process_knowledge_units`、`process_knowledge_evidence`、`process_knowledge_chunks`、`process_knowledge_jobs`、`process_knowledge_state`；权限 `process_knowledge:read/manage`。

- [ ] 编写 migration 失败测试，检查约束、RLS、GIN/trigram/范围索引和权限。
- [ ] 运行 `go test ./internal/processknowledge ./internal/cleaning -run Migration -v`，确认缺少 migration。
- [ ] 实现 migration，状态枚举与规格完全一致，并为 `pg_trgm` 建立关键词索引。
- [ ] 重跑测试。

### Task 3: Supersession 逻辑项目继承

**Files:**
- Modify: `server/internal/projectattribution/repository.go`
- Modify: `server/internal/projectattribution/rules.go`
- Modify: `server/internal/projectattribution/rules_test.go`
- Modify: `server/internal/projectattribution/repository_test.go`

**Interfaces:**
- Produces: `ResolveSupersededProject(replacementEventID)`，替代事件在没有更强证据时继承旧事件当前逻辑项目。

- [ ] 新增失败测试：原事件属于 `safe`，替代事件原始项目为 `Codex`，结果仍为 `safe` 且原因码为 `superseded_event_inheritance`。
- [ ] 运行项目归属测试确认失败。
- [ ] 在回填候选加载和规则优先级中加入继承证据；显式精确绑定仍可覆盖继承。
- [ ] 重跑 `go test ./internal/projectattribution -v`。

### Task 4: 会话重建与轮次分类

**Files:**
- Create: `server/internal/processknowledge/model.go`
- Create: `server/internal/processknowledge/session.go`
- Create: `server/internal/processknowledge/session_test.go`
- Create: `server/internal/processknowledge/classify.go`
- Create: `server/internal/processknowledge/classify_test.go`

**Interfaces:**
- Produces: `AssembleSessions([]SourceTurn) []SessionDraft`、`ClassifyTurn(SourceTurn) StatementKind`。
- 精确键：tenant、device、logical project、AI tool、session ID；缺少 ID 才允许低置信度时间推断。

- [ ] 编写失败测试，覆盖相同时间不同 session 不串联、人工问题/确认/否定、AI 探索/最终结论和工具验证。
- [ ] 运行测试确认类型和函数不存在。
- [ ] 实现确定性会话组装与轮次分类；长 AI 回复和结构化完成标记优先识别为最终回答。
- [ ] 重跑测试。

### Task 5: 知识提取、分块与证据状态

**Files:**
- Create: `server/internal/processknowledge/extract.go`
- Create: `server/internal/processknowledge/extract_test.go`
- Create: `server/internal/processknowledge/chunk.go`
- Create: `server/internal/processknowledge/chunk_test.go`

**Interfaces:**
- Produces: `ExtractKnowledge(SessionDraft) []KnowledgeDraft`、`ChunkKnowledge(KnowledgeDraft, min, max int) []ChunkDraft`。
- 用户问题成为 `problem/intent`；只有 AI 最终回答产生 `conclusion`；确认更新决策状态；验证结果更新验证状态。

- [ ] 编写失败测试：单独用户问题不生成结论；55,000 字后半段 EDR 内容可形成块；API 名称不在中间截断。
- [ ] 运行测试确认失败。
- [ ] 实现最小确定性提取器、证据链和 600-1,000 字语义分块。
- [ ] 重跑测试。

### Task 6: Repository、任务与增量 Worker

**Files:**
- Create: `server/internal/processknowledge/repository.go`
- Create: `server/internal/processknowledge/repository_test.go`
- Create: `server/internal/processknowledge/service.go`
- Create: `server/internal/processknowledge/service_test.go`
- Create: `server/internal/processknowledge/worker.go`
- Create: `server/internal/processknowledge/worker_test.go`
- Modify: `server/cmd/aetheris-server/main.go`
- Modify: `server/internal/config/config.go`
- Modify: `server/internal/config/config_test.go`

**Interfaces:**
- Produces: `SyncSessions`、`ExtractDirtySessions`、`StartBackfill`、`ActivateVersion`、`RollbackVersion`、`Summary`、`ListUnits`、`GetUnit`。
- Worker 使用持久游标和有界批次，失败不阻塞 ingest。

- [ ] 编写 repository/service/worker 失败测试，覆盖幂等、dirty revision、dry-run/apply/resume/activate/rollback。
- [ ] 运行测试确认失败。
- [ ] 实现 PostgreSQL repository 与 Worker，并增加 `PROCESS_KNOWLEDGE_*` 配置。
- [ ] 重跑包测试和 config 测试。

### Task 7: 独立知识向量与混合检索

**Files:**
- Create: `server/internal/processknowledge/search.go`
- Create: `server/internal/processknowledge/search_test.go`
- Create: `server/internal/processknowledge/indexer.go`
- Create: `server/internal/processknowledge/indexer_test.go`
- Modify: `server/internal/retrieval/query_service.go`
- Modify: `server/internal/retrieval/model.go`
- Modify: `server/internal/retrieval/query_test.go`
- Modify: `server/internal/config/config.go`
- Modify: `server/cmd/aetheris-server/main.go`

**Interfaces:**
- Produces: `KnowledgeSearcher.Search(ctx, KnowledgeQuery) ([]KnowledgeHit, error)`。
- 双路召回使用 Qdrant 稠密向量与 PostgreSQL trigram；Go 使用 RRF、知识单元去重、会话配额和验证状态优先。

- [ ] 编写失败测试，覆盖精确 `WFP` 命中、语义命中、RRF、项目隔离、同会话去重和 verified 优先。
- [ ] 运行测试确认失败。
- [ ] 实现 `aetheris_process_knowledge_v1` 索引器和搜索器。
- [ ] 将 `knowledge_scope=project_process` 查询接入 QueryService，旧活动 RAG 继续兼容。
- [ ] 重跑 processknowledge/retrieval 测试。

### Task 8: 两阶段回答与动态生成预算

**Files:**
- Modify: `server/internal/retrieval/generator.go`
- Modify: `server/internal/retrieval/generator_test.go`
- Modify: `server/internal/retrieval/answer.go`
- Modify: `server/internal/retrieval/answer_test.go`
- Modify: `model-gateway/aetheris_model_gateway/protocols.py`
- Modify: `model-gateway/aetheris_model_gateway/providers/ollama.py`
- Modify: `model-gateway/tests/test_providers.py`

**Interfaces:**
- Produces: `plan_rag_answer` 与 `rag_answer` 两个内部任务；analysis 预算 2,048 token，direct 256，reason/procedure 768。
- 本地引用带 `knowledge_id/source_kind/validation_state/applicability`。

- [ ] 编写 Go/Python 失败测试，覆盖先计划后回答、动态 token、用户问题不能作为事实、验证冲突优先。
- [ ] 运行测试确认失败。
- [ ] 实现计划结构、模型调用和服务端校验；生成失败使用已验证知识确定性保底。
- [ ] 重跑 Go retrieval 与 Model Gateway 测试。

### Task 9: Admin API 与 OpenAPI

**Files:**
- Create: `server/internal/httpapi/process_knowledge_handlers.go`
- Create: `server/internal/httpapi/process_knowledge_handlers_test.go`
- Modify: `server/internal/httpapi/router.go`
- Modify: `server/internal/httpapi/rag_handlers.go`
- Modify: `contracts/openapi.yaml`
- Modify: `server/cmd/aetheris-server/main.go`

**Interfaces:**
- Produces: `/admin/process-knowledge/summary|units|jobs|versions`；RAG 请求支持 `logical_project_id`、`knowledge_scope`、显式全部项目。

- [ ] 编写失败的 handler 与契约测试，覆盖权限、分页、项目必选、任务状态和引用字段。
- [ ] 运行测试确认失败。
- [ ] 实现 handler、路由、权限和 OpenAPI。
- [ ] 重跑 httpapi 测试并解析 OpenAPI YAML。

### Task 10: Admin Web 过程知识与查询体验

**Files:**
- Create: `admin-web/src/pages/ProcessKnowledgePage.tsx`
- Create: `admin-web/src/pages/ProcessKnowledgePage.test.tsx`
- Modify: `admin-web/src/pages/RAGQueryPage.tsx`
- Modify: `admin-web/src/pages/RAGQueryPage.test.tsx`
- Modify: `admin-web/src/api/types.ts`
- Modify: `admin-web/src/api/client.ts`
- Modify: `admin-web/src/App.tsx`
- Modify: `admin-web/src/styles.css`

**Interfaces:**
- 查询默认选择逻辑项目；“全部项目”必须显式选择。
- 页面展示知识状态、来源构成、覆盖主题、适用条件和证据链，不展示项目能力清单。

- [ ] 编写失败组件测试，覆盖项目必选、显式全部项目、验证标签、引用展开、回填和空/错/加载状态。
- [ ] 运行 Vitest 确认失败。
- [ ] 实现类型、API、页面、导航和样式。
- [ ] 重跑前端测试、类型检查和构建。

### Task 11: 回填 CLI、评测数据集与生产验收

**Files:**
- Modify: `server/cmd/aetheris-admin/main.go`
- Modify: `server/cmd/aetheris-admin/main_test.go`
- Create: `server/internal/processknowledge/evaluation.go`
- Create: `server/internal/processknowledge/evaluation_test.go`
- Create: `docs/operations/process-knowledge-rag-acceptance.md`
- Modify: `docs/operations/server-deployment.md`
- Modify: `docs/operations/server-operations.md`
- Modify: `docs/operations/implementation-log.md`

**Interfaces:**
- Produces: `aetheris-admin process-knowledge-backfill`、`process-knowledge-export-eval`；训练候选仅导出 accepted+verified+active。

- [ ] 编写失败测试，固定 EDR 问题必须覆盖架构主题、引用过程知识且不输出能力清单。
- [ ] 运行测试确认失败。
- [ ] 实现 CLI、评测导出和运维文档。
- [ ] 运行 Python、Go、Model Gateway、Admin Web、VS Code 全量测试与构建。
- [ ] 部署到测试/生产前执行 migration、dry-run、shadow 对账、canary 和回滚演练。
