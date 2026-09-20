# 平台共享知识与全局智能查询实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**目标：** 补齐可观察的 AI 搜索、工具结果和显式分析采集，建立租户私有过程知识与平台公共知识双层架构，并让智能查询默认汇总当前租户全部项目知识和已认证公共知识。

**架构：** Windows Core 将结构化 AI 过程写入不可变事件；现有 `processknowledge` 包继续维护租户私有知识，新建 `publicknowledge` 包负责脱敏候选、独立性判断、审核、发布和公共索引。检索层通过组合搜索器并行读取租户私有 Qdrant 集合和公共 Qdrant 集合，生成带 `tenant_private`、`platform_public` 或 `model_general` 来源标记的答案。

**技术栈：** Python 3.11、Go 1.22、PostgreSQL、Qdrant、React 18、TypeScript 5.7、Vite、Vitest。

**规格：** `docs/superpowers/specs/2026-09-20-platform-shared-knowledge-design.md`

## 全局约束

- 原始事件不可变；清洗、知识抽取和公共发布均使用新 revision。
- 普通租户不能读取其他租户的原始事件、会话、项目、设备、人员、路径、代码或私有知识 ID。
- 公共知识必须通过平台认证后才能写入公共 Qdrant 集合并参与查询。
- AI 搜索和工具过程只采集可观察内容，不采集隐藏推理过程。
- AI 命令只保存命令类型、安全摘要和设备内 HMAC。
- 智能查询不要求选择项目，项目仅用于私有证据归属和管理诊断。
- PostgreSQL 是知识正文、版本和发布状态的唯一事实来源。
- 所有 Ollama 抽取、公共知识处理和历史回填均为异步任务。
- 不引入员工排名、绩效评分或黑盒可信度总分。

---

### Task 1: 采集 AI 搜索、工具结果和显式分析摘要

**Files:**
- Modify: `src/aetheris/adapters/ai_sessions.py`
- Modify: `tests/test_ai_session_adapter.py`
- Modify: `contracts/aetheris-event-v1.schema.json`
- Modify: `schemas/aetheris-event-v1.schema.json`

**Interfaces:**
- Consumes: Codex JSONL `response_item.payload` 与 Claude Code `message.content` 结构。
- Produces: `AISessionAdapter.collect() -> list[dict]` 中新增 `ai.search_query`、`ai.search_result`、`ai.tool_result`、`ai.reasoning_summary` 记录；每条记录包含可关联的 `session_id`、`message_id` 或 `call_id`。

- [x] **Step 1: 为 Codex 与 Claude Code 新事件写失败测试**

在 `tests/test_ai_session_adapter.py` 增加固定 JSONL 样例，断言：

```python
assert [item["event_type"] for item in records] == [
    "ai.search_query", "ai.search_result", "ai.reasoning_summary", "ai.tool_result"
]
assert records[0]["payload"]["call_id"] == "search-1"
assert records[1]["payload"]["source_domain"] == "learn.microsoft.com"
assert "C:\\Users\\" not in json.dumps(records, ensure_ascii=False)
```

同时增加 Shell 工具回归断言：`ai.tool_result` 不得包含完整命令，只能包含 `tool_kind`、`result_state`、`safe_summary` 和 `result_hash`。

- [x] **Step 2: 运行适配器测试并确认新用例失败**

Run: `python -m pytest tests/test_ai_session_adapter.py -q`

Expected: FAIL，原因是新事件类型尚未生成。

- [x] **Step 3: 实现结构化辅助事件解析**

在 `AISessionAdapter` 中新增：

```python
def _parse_observable_ai_process(self, path: Path, item: dict, source_offset: int) -> list[dict]: ...
def _safe_external_url(raw: str) -> tuple[str, str]: ...
def _result_hash(value: str) -> str: ...
```

Codex 支持 `web_search_call`、`web_search_result`、`reasoning` 及非 Shell `function_call_output`；Claude Code 支持 `WebSearch`、`WebFetch`、`Grep`、`Read` 的 `tool_use` 与 `tool_result`。所有正文先经过 `Redactor.redact()`，URL 去除凭据和查询参数，网页只保留标题、域名、安全摘要与哈希。

- [x] **Step 4: 更新两份事件 Schema 说明并运行测试**

Schema 的 `event_type.description` 明确列出四种新增事件；不改变 schema version 和通用 payload 结构。

Run: `python -m pytest tests/test_ai_session_adapter.py tests/test_schema_and_queue_stats.py -q`

Expected: PASS。

- [x] **Step 5: 提交客户端采集改动**

```bash
git add src/aetheris/adapters/ai_sessions.py tests/test_ai_session_adapter.py contracts/aetheris-event-v1.schema.json schemas/aetheris-event-v1.schema.json
git commit -m "feat: capture observable ai research activity"
```

### Task 2: 扩展租户私有过程知识证据

**Files:**
- Modify: `server/internal/processknowledge/model.go`
- Modify: `server/internal/processknowledge/classify.go`
- Modify: `server/internal/processknowledge/extract.go`
- Modify: `server/internal/processknowledge/repository.go`
- Modify: `server/internal/processknowledge/classify_test.go`
- Modify: `server/internal/processknowledge/extract_test.go`
- Modify: `server/internal/processknowledge/repository_test.go`
- Modify: `server/internal/cleaning/model.go`
- Modify: `server/internal/cleaning/rules.go`
- Modify: `server/internal/cleaning/rules_test.go`

**Interfaces:**
- Consumes: 清洗事实事件类型 `ai.message`、`ai.tool_call`、`ai.search_query`、`ai.search_result`、`ai.tool_result`、`ai.reasoning_summary`。
- Produces: 新 `StatementKind` 常量 `SearchQuery`、`SearchResult`、`ReasoningSummary`，以及 `KnowledgeDraft` 中可检索的 rationale、alternatives、applicability 和 caveats。

- [x] **Step 1: 写分类与抽取失败测试**

构造一个包含人工问题、搜索、分析摘要、工具结果、最终回答和成功运行验证的 `SessionDraft`，断言：

```go
if units[0].ValidationState != "verified" { t.Fatal(...) }
if !strings.Contains(units[0].Rationale, "官方文档") { t.Fatal(...) }
if evidenceKinds(units[0].Evidence)["external_source"] != 1 { t.Fatal(...) }
```

另加测试确保搜索结果本身不能在没有最终回答时生成知识单元。

- [x] **Step 2: 运行过程知识单元测试并确认失败**

Run: `cd server && go test ./internal/processknowledge -run 'Test(Classify|Extract)'`

Expected: FAIL，缺少新 StatementKind 和证据提取。

- [x] **Step 3: 扩展分类、抽取和来源查询**

`sourceTurnsSQL` 将允许事件扩展为：

```sql
f.event_type IN (
  'ai.message','ai.tool_call','ai.search_query','ai.search_result',
  'ai.tool_result','ai.reasoning_summary'
)
```

分类规则只把显式 `ai.reasoning_summary` 归为分析摘要；搜索和工具结果写入证据，不单独形成结论。抽取器将这些内容写入 rationale、alternatives、applicability 或 validation，并保持最终回答为 conclusion 的主要来源。

- [x] **Step 4: 运行过程知识包测试**

Run: `cd server && go test ./internal/processknowledge`

Expected: PASS。

- [x] **Step 5: 提交私有知识扩展**

```bash
git add server/internal/processknowledge
git commit -m "feat: enrich private process knowledge evidence"
```

### Task 3: 创建公共知识数据库与权限模型

**Files:**
- Create: `server/migrations/021_public_knowledge.sql`
- Create: `server/internal/publicknowledge/migration_test.go`
- Modify: `server/internal/db/migrate_test.go`

**Interfaces:**
- Consumes: `process_knowledge_units` 和 `process_knowledge_evidence` 的稳定 revision。
- Produces: `public_knowledge_units`、`public_knowledge_revisions`、`public_knowledge_sources`、`public_knowledge_reviews`、`public_knowledge_conflicts`、`public_knowledge_jobs` 表和五个新权限。

- [ ] **Step 1: 写 migration 合同测试**

测试读取 `021_public_knowledge.sql`，断言包含：

```go
required := []string{
    "CREATE TABLE IF NOT EXISTS public_knowledge_units",
    "CREATE TABLE IF NOT EXISTS public_knowledge_revisions",
    "CREATE TABLE IF NOT EXISTS public_knowledge_sources",
    "CREATE TABLE IF NOT EXISTS public_knowledge_reviews",
    "CREATE TABLE IF NOT EXISTS public_knowledge_conflicts",
    "CREATE TABLE IF NOT EXISTS public_knowledge_jobs",
    "knowledge:certify_public",
}
```

同时断言公共 source 表没有普通租户公共读取视图，发布 revision 具有唯一 canonical hash 和状态约束。

- [ ] **Step 2: 运行 migration 测试并确认失败**

Run: `cd server && go test ./internal/publicknowledge ./internal/db`

Expected: FAIL，因为 migration 和包尚不存在。

- [ ] **Step 3: 实现 migration**

状态约束使用规格中的精确值；`public_knowledge_sources` 保存 `source_tenant_id` 与私有知识 revision，但匿名计数物化到 `public_knowledge_revisions`。权限映射：

- platform_admin：全部五项权限。
- tenant_admin：confirm_source、verify、diagnose。
- analyst：confirm_source、verify、diagnose。
- member：无公共治理写权限。

为 canonical hash、publication state、validation state、当前 revision、任务状态和来源私有知识建立索引。

- [ ] **Step 4: 运行 migration 合同与全量迁移测试**

Run: `cd server && go test ./internal/publicknowledge ./internal/db`

Expected: PASS。

- [ ] **Step 5: 提交数据库模型**

```bash
git add server/migrations/021_public_knowledge.sql server/internal/publicknowledge/migration_test.go server/internal/db/migrate_test.go
git commit -m "feat: add public knowledge database model"
```

### Task 4: 实现公共知识领域、脱敏与审核状态机

**Files:**
- Create: `server/internal/publicknowledge/model.go`
- Create: `server/internal/publicknowledge/sanitize.go`
- Create: `server/internal/publicknowledge/candidate.go`
- Create: `server/internal/publicknowledge/service.go`
- Create: `server/internal/publicknowledge/repository.go`
- Create: `server/internal/publicknowledge/sanitize_test.go`
- Create: `server/internal/publicknowledge/candidate_test.go`
- Create: `server/internal/publicknowledge/service_test.go`

**Interfaces:**
- Produces: `Service.List`、`Service.Get`、`Service.Confirm`、`Service.Verify`、`Service.Certify`、`Service.Reject`、`Service.Suspend`、`Service.Withdraw`、`Service.StartBuild`。
- Produces: `BuildCandidate(PrivateKnowledge) (Candidate, error)` 和 `SanitizePublicText(string) (string, Report)`。
- Consumes: Repository 以事务完成 revision 检查、审核日志和状态迁移。

- [ ] **Step 1: 写脱敏与候选独立性失败测试**

覆盖 Windows 路径、用户名、邮箱、IPv4、内部 URL、Bearer token、项目名和代码围栏。断言：

```go
safe, report := SanitizePublicText(raw)
if strings.Contains(safe, `E:\project\safe`) || report.Blocked { t.Fatal(...) }
```

候选测试断言同一内容哈希、同一外部 URL 或镜像批次不会增加独立租户计数，不兼容 applicability 会生成冲突。

- [ ] **Step 2: 写审核状态机失败测试**

覆盖：candidate -> pending_review -> published、revision 冲突、未认证禁止发布、published -> suspended、published -> withdrawn、撤回幂等。

Run: `cd server && go test ./internal/publicknowledge`

Expected: FAIL。

- [ ] **Step 3: 实现模型、脱敏器和候选构建器**

公共 DTO 不包含 `SourceTenantID`。内部来源使用独立 `SourceLink` 类型，不能嵌入公共响应对象。`canonical_hash` 基于二次脱敏后的 topic、conclusion、applicability 和 caveats 计算 SHA-256。

- [ ] **Step 4: 实现 Service 与 PostgreSQL Repository**

所有状态写操作接受：

```go
type ReviewCommand struct {
    KnowledgeID string
    ExpectedRevision int
    ActorID string
    Action string
    Reason string
}
```

Repository 在单事务内锁定知识行、验证 revision、插入 review、更新状态。普通详情查询只返回匿名来源数和当前租户自己的 `PrivateEvidenceRef`。

- [ ] **Step 5: 运行领域测试**

Run: `cd server && go test ./internal/publicknowledge`

Expected: PASS。

- [ ] **Step 6: 提交公共知识领域**

```bash
git add server/internal/publicknowledge
git commit -m "feat: add governed public knowledge domain"
```

### Task 5: 实现公共候选异步任务与双索引发布

**Files:**
- Create: `server/internal/publicknowledge/worker.go`
- Create: `server/internal/publicknowledge/indexer.go`
- Create: `server/internal/publicknowledge/search.go`
- Create: `server/internal/publicknowledge/worker_test.go`
- Create: `server/internal/publicknowledge/indexer_test.go`
- Create: `server/internal/publicknowledge/search_test.go`
- Modify: `server/internal/retrieval/model.go`
- Modify: `server/internal/retrieval/qdrant.go`
- Modify: `server/internal/retrieval/qdrant_test.go`

**Interfaces:**
- Produces: `Worker.RunOnce(context.Context, int) (bool, error)`。
- Produces: `Indexer.RunOnce(context.Context, int) (IndexResult, error)`，仅索引 platform_certified + published revision。
- Produces: `Searcher.Search(context.Context, retrieval.KnowledgeQuery) ([]retrieval.KnowledgeHit, error)`，公共结果的 `SourceScope` 为 `platform_public`。
- Extends: `VectorIndex.Delete(context.Context, []string) error` 用于撤回公共向量。

- [ ] **Step 1: 写任务、索引和撤回失败测试**

断言未认证候选不进入 PendingPublicChunks；已认证 revision 写入公共集合后才变为 indexed；撤回先使数据库不可查，再调用向量删除；向量删除失败时数据库仍保持 withdrawn。

- [ ] **Step 2: 扩展 Qdrant 删除接口并验证请求**

实现：

```go
func (client *QdrantClient) Delete(ctx context.Context, ids []string) error
```

请求 `/collections/{collection}/points/delete?wait=true`，body 为 `{"points": ids}`。

Run: `cd server && go test ./internal/retrieval -run Qdrant`

Expected: PASS。

- [ ] **Step 3: 实现候选任务与公共 Indexer**

候选任务按私有知识 revision 游标读取；只把符合资格的知识写为 candidate/pending_review。Indexer 使用独立 Qdrant 客户端，payload 不带 tenant、project、device、session 或 event ID。

- [ ] **Step 4: 实现公共 Searcher**

同时使用公共向量与 PostgreSQL 关键词候选，RRF 去重后返回最多请求 limit；只读取 published 且当前 revision 一致的记录。

- [ ] **Step 5: 运行公共知识与检索测试**

Run: `cd server && go test ./internal/publicknowledge ./internal/retrieval`

Expected: PASS。

- [ ] **Step 6: 提交异步发布和索引**

```bash
git add server/internal/publicknowledge server/internal/retrieval
git commit -m "feat: publish certified knowledge to public index"
```

### Task 6: 将私有检索改为全项目并组合公共检索

**Files:**
- Modify: `server/internal/processknowledge/search.go`
- Modify: `server/internal/processknowledge/search_test.go`
- Create: `server/internal/retrieval/composite_knowledge.go`
- Create: `server/internal/retrieval/composite_knowledge_test.go`
- Modify: `server/internal/retrieval/model.go`
- Modify: `server/internal/retrieval/query_service.go`
- Modify: `server/internal/retrieval/query_test.go`
- Modify: `server/internal/retrieval/answer.go`
- Modify: `server/internal/retrieval/answer_test.go`

**Interfaces:**
- Produces: `CompositeKnowledgeSearcher.Search` 并行合并 private 与 public searcher。
- Extends: `KnowledgeHit` 新增 `SourceScope`、`PublicKnowledgeID`、`Revision`、`AnonymousSourceTenantCount`。
- Changes: `QueryInput` 默认 `knowledge_scope=tenant_and_public`，不再自动选择单一项目。

- [ ] **Step 1: 写全项目检索失败测试**

给三个逻辑项目各放置候选，断言结果包含多个项目且每项目最多三条；同一知识内容跨项目只保留最高质量项。

- [ ] **Step 2: 写组合检索与优先级失败测试**

输入一个私有 verified、一个公共 platform_certified、一个私有 unverified 和一个 model_general 缺口，断言顺序为：私有已验证、公共已认证、私有未验证；引用保留正确 `source_scope`。

- [ ] **Step 3: 移除自动单项目路由并增加项目配额**

保留 `SelectAutomaticProject` 仅用于兼容诊断测试，不在默认 Search 路径调用。`FuseCandidates` 增加 `projectCount`，同项目最多三条，并保证不同 topic 的最低覆盖。

- [ ] **Step 4: 实现组合检索器和引用字段**

并行调用两个 searcher；公共搜索失败时返回私有结果并记录降级，私有搜索失败时可返回公共结果。任何失败都不能改用无 tenant filter 的私有检索。

- [ ] **Step 5: 更新回答校验**

回答计划必须区分 `tenant_private`、`platform_public` 和 `model_general`。私有已验证内容与公共内容冲突时优先选择 applicability 匹配的私有结论，并创建可持久化的冲突通知接口调用。

- [ ] **Step 6: 运行查询与回答测试**

Run: `cd server && go test ./internal/processknowledge ./internal/retrieval`

Expected: PASS。

- [ ] **Step 7: 提交全局组合检索**

```bash
git add server/internal/processknowledge server/internal/retrieval
git commit -m "feat: search tenant and public knowledge globally"
```

### Task 7: 接入配置、后台任务和知识治理 API

**Files:**
- Modify: `server/internal/config/config.go`
- Modify: `server/internal/config/config_test.go`
- Modify: `server/cmd/aetheris-server/main.go`
- Modify: `server/internal/httpapi/router.go`
- Create: `server/internal/httpapi/public_knowledge_handlers.go`
- Create: `server/internal/httpapi/public_knowledge_handlers_test.go`
- Modify: `contracts/openapi.yaml`
- Modify: `contracts/admin-api.types.ts`

**Interfaces:**
- Adds config: `PUBLIC_KNOWLEDGE_COLLECTION` 默认 `aetheris_public_knowledge_v1`，`PUBLIC_KNOWLEDGE_INTERVAL` 默认 300 秒，`PUBLIC_KNOWLEDGE_BATCH` 默认 25。
- Adds dependency: `PublicKnowledge PublicKnowledgeService`。
- Adds endpoints under `/api/v1/admin/public-knowledge/` as specified in the design.

- [ ] **Step 1: 写配置与 API 权限失败测试**

断言默认公共集合与有界任务配置；tenant_admin 不能 certify 或 withdraw；platform_admin 有对应 permission 时可以；未认证知识详情不会返回其他租户 source ID。

- [ ] **Step 2: 实现配置和主程序装配**

主程序创建独立公共 Qdrant client、public repository/service/worker/indexer/searcher，并用 `retrieval.NewCompositeKnowledgeSearcher(private, public)` 注入 QueryService。

- [ ] **Step 3: 实现治理 handlers**

所有 POST body 包含 `expected_revision` 和 `reason`；缺失 reason、revision 冲突或非法状态转换返回 400/409，不返回数据库错误正文。

- [ ] **Step 4: 更新 OpenAPI 和共享 TypeScript 合同**

合同包含公共知识列表、详情、审核动作、任务状态和智能查询新增 citation 字段。

- [ ] **Step 5: 运行服务端测试**

Run: `cd server && go test ./...`

Expected: PASS。

- [ ] **Step 6: 提交服务装配和 API**

```bash
git add server contracts
git commit -m "feat: expose governed public knowledge api"
```

### Task 8: 构建 Admin Web 知识治理页面并简化智能查询

**Files:**
- Create: `admin-web/src/pages/PublicKnowledgePage.tsx`
- Create: `admin-web/src/pages/PublicKnowledgePage.test.tsx`
- Modify: `admin-web/src/pages/RAGQueryPage.tsx`
- Modify: `admin-web/src/pages/RAGQueryPage.test.tsx`
- Modify: `admin-web/src/api/types.ts`
- Modify: `admin-web/src/api/client.ts`
- Modify: `admin-web/src/api/client.test.ts`
- Modify: `admin-web/src/App.tsx`
- Modify: `admin-web/src/App.test.tsx`
- Modify: `admin-web/src/styles.css`

**Interfaces:**
- Produces: “知识治理”导航页，显示队列、匿名覆盖、版本、验证与发布状态，并提供基于权限的确认、验证、认证、暂停和撤回操作。
- Changes: 智能查询请求默认 `{question, knowledge_scope: 'tenant_and_public'}`，页面不再显示项目、设备和日期必选项。

- [ ] **Step 1: 写知识治理页面失败测试**

用 Testing Library 断言六个队列、匿名来源数、认证按钮权限、revision 提交和冲突错误提示。

- [ ] **Step 2: 写智能查询全局范围失败测试**

断言页面只有问题输入和开始查询主操作；请求包含 `tenant_and_public`；引用卡片显示“租户私有”“平台公共”或“模型通用知识”，不显示其他租户或项目路径。

- [ ] **Step 3: 扩展 API 类型和客户端**

新增 `PublicKnowledgeUnit`、`PublicKnowledgeRevision`、`PublicKnowledgeReview`、`PublicKnowledgePage` 和 `PublicKnowledgeCommand`；所有治理 mutation 使用统一 `expected_revision` 与 `reason`。

- [ ] **Step 4: 实现知识治理页面和导航**

使用现有 panel、table、drawer、badge 和 notice 组件风格，不嵌套卡片。操作期间禁用对应按钮并显示明确状态，失败后保留用户填写的审核理由。

- [ ] **Step 5: 简化智能查询并更新引用展示**

移除普通查询的范围表单；保留异步索引、检索、生成进度。公共引用点击打开公共知识详情，私有引用继续打开本租户证据。

- [ ] **Step 6: 运行前端测试与构建**

Run: `cd admin-web && npm test -- --run`

Expected: PASS。

Run: `cd admin-web && npm run build`

Expected: PASS。

- [ ] **Step 7: 提交 Admin Web**

```bash
git add admin-web
git commit -m "feat: add public knowledge governance ui"
```

### Task 9: 历史回填、运行配置和运维验收

**Files:**
- Modify: `deploy/docker-compose.yml`
- Modify: `deploy/systemd/aetheris-server.service`
- Modify: `deploy/env.example`
- Create: `docs/operations/platform-shared-knowledge-rollout.md`
- Create: `scripts/backfill-public-knowledge.ps1`
- Create: `scripts/backfill-public-knowledge.sh`
- Modify: `tests/test_model_provisioning.py`
- Add tests beside affected deploy/script tests selected by `rg --files tests`.

**Interfaces:**
- Produces: 可暂停、续跑的私有知识回填与公共候选生成入口。
- Configures: 公共 Qdrant 集合、300 秒后台间隔、25 条批量、独立 CPU/并发限制。

- [ ] **Step 1: 写部署配置失败测试**

断言 env 示例和 systemd 环境包含公共集合与任务参数，且公共后台任务批量小于私有知识批量。

- [ ] **Step 2: 更新部署和配置文件**

公共任务与用户查询共享 WorkloadGate，但使用最低优先级；文档给出 dry-run、candidate build、人工认证、shadow 查询、activate 和 rollback 的确切命令。

- [ ] **Step 3: 实现可恢复回填脚本**

脚本必须接受 server URL、管理员凭据、mode、version 和 batch 参数；轮询 job，打印 scanned/candidate/conflict/failed；非零 failed 或 API 错误时退出非零。

- [ ] **Step 4: 运行部署相关测试**

Run: `python -m pytest tests/test_model_provisioning.py tests/test_server_workspace.py -q`

Expected: PASS。

- [ ] **Step 5: 提交运维能力**

```bash
git add deploy scripts docs/operations tests
git commit -m "ops: add shared knowledge rollout workflow"
```

### Task 10: 端到端验证与版本交付

**Files:**
- Modify: `tests/test_real_adapter_acceptance.py`
- Modify: `README.md`
- Modify: `src/aetheris/version.py`
- Modify: `admin-web/package.json`

**Interfaces:**
- Verifies: 采集、私有知识、公共候选、认证、双索引、全局查询、撤回和安全降级的完整路径。
- Produces: 统一版本号和中文运行说明。

- [ ] **Step 1: 增加端到端验收用例**

构造 tenant-a 的 verified 私有知识、tenant-b 的独立印证和 tenant-c 的查询。断言认证前 tenant-c 无法命中；认证后可以命中公共 ID；tenant-c 无法获取 tenant-a/b 的原始来源；撤回后新查询不再引用该公共 ID。

- [ ] **Step 2: 运行 Python、Go 和前端全量测试**

Run: `python -m pytest -q`

Run: `cd server && go test ./...`

Run: `cd admin-web && npm test -- --run`

Expected: 全部 PASS。

- [ ] **Step 3: 构建全部交付物**

Run: `cd admin-web && npm run build`

Run: `cd server && go build ./cmd/aetheris-server ./cmd/aetheris-admin ./cmd/aetheris-migrate`

Run: `python -m build`

Expected: 全部成功，版本号一致。

- [ ] **Step 4: 执行安全与规格验收扫描**

运行仓库现有敏感字面量检查，并用 `rg` 确认公共 DTO 和 Qdrant payload 不含 `source_tenant_id`、设备、项目和原始事件字段。

Run: `python -m pytest tests/test_no_sensitive_literals.py -q`

Expected: PASS。

- [ ] **Step 5: 提交版本交付**

```bash
git add tests README.md src/aetheris/version.py admin-web/package.json
git commit -m "release: deliver platform shared knowledge v1"
```
