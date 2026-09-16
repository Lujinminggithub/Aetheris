# Aetheris 统一活动与本地 RAG 设计

**日期：** 2026-09-07  
**状态：** 已实施并完成生产验收  
**范围：** Windows Core、Go Server、Model Gateway、Qdrant、Admin Web  

## 1. 目标

1. 将 AI、终端、IDE、浏览器、版本控制等数据统一为“活动记录”，AI 不再作为独立一级数据页面。
2. 修复 AI 工作目录无法映射时直接丢弃数据的问题；无法安全映射时归入 `Codex`、`Claude Code`、`Cursor` 等逻辑项目。
3. 建立真实可运行的本地 RAG：清洗事实生成检索文档，Ollama 生成向量和回答，Qdrant 保存向量，PostgreSQL 保存权限、文档、任务和引用。
4. Admin Web 提供独立“智能查询”入口，查询异步执行并显示进度，答案必须带可打开的事实/事件引用。

## 2. 当前问题与根因

- Admin Web 同时存在“事件”和“AI 交互”，前者只取最近 100 条原始事件，后者使用独立聚合接口，形成两套数据视图。
- 本机 Codex 历史中 7,345 个会话元数据有 3,991 个 cwd 位于授权项目根之外；Core 的 `event_from_record()` 对这些记录抛出异常并跳过。
- 生产 `event_embeddings` 为 0，Go Server 使用 `DisabledVectorIndex`，没有检索 worker、查询 API 或查询页面。
- Ollama 已能生成回答，但初始只有 `qwen3:4b-instruct`；RAG 增加多语言及代码检索模型 `embeddinggemma`。
- PostgreSQL 14 没有 pgvector，向量层按既定边界使用独立 Qdrant。

## 3. 总体架构

```text
Windows Core
  AI cwd 本地归因
       |
       v
events（不可变原始证据）
       |
       v
Cleaning Worker -> clean_event_facts（统一活动事实）
       |                         |
       |                         +-> GET /admin/activities
       v
Retrieval Indexer -> retrieval_documents(PostgreSQL)
       |              + Qdrant 向量
       v
POST /admin/rag/queries -> 异步任务
       |  embed query -> Qdrant top-k -> RBAC复核
       |  evidence -> Model Gateway -> Ollama
       v
答案 + citations -> Admin Web 智能查询
```

## 4. AI 项目归因

Core 只在本地使用 cwd 做归因，不上传无法授权的完整路径：

1. cwd 位于授权 Git/SVN 项目内：使用稳定项目 ID，展示仓库末级目录名。
2. cwd 不存在、位于临时目录或授权范围外：生成按工具稳定的逻辑项目 ID，例如 `project-tool-codex`，标签为 `Codex`。
3. 支持的逻辑标签：`Codex`、`Claude Code`、`Cursor`、`GitHub Copilot`，未知 AI 工具显示规范化工具名。
4. payload 只写 `project_label` 和 `project_attribution=authorized_root|tool_fallback`，不写 fallback cwd。
5. 事件 ID 继续按原始结构化记录和源偏移计算，使已上传记录保持幂等；历史重放只补齐此前被丢弃的记录。
6. 非 AI 事件仍必须属于授权项目，不扩大终端、IDE 或浏览器采集范围。

## 5. 统一活动事实

`clean_event_facts` 增加普通活动类型，并为查询保存确定性维度：

- `fact_type`：`ai_interaction`、`terminal_operation`、`activity`、`command_fragment`。
- `event_type`、`source`、`activity_type`：`ai`、`terminal`、`ide`、`browser`、`version_control`、`other`。
- AI 消息保留 `message_role`；AI 工具只使用 `command_type` 和 `command_summary`。
- Git、IDE、浏览器等原始事件生成一对一 `activity` 事实。
- `quarantined`、`excluded_from_effectiveness=true` 不进入默认活动列表和 RAG。
- 原始事件不修改；事实继续保留规则版本和 `source_event_ids`。

Admin Web 删除独立“AI 交互”和“事件”导航，替换为一个“活动记录”入口。旧 `/api/v1/admin/ai-interactions` 暂时保留兼容，但不再由页面调用。

`GET /api/v1/admin/activities` 支持：

- `from`、`to`
- `device_id`、`project_id`
- `activity_type`、`message_role`
- `limit`、`offset`

响应包含设备、项目、活动类型计数、分页活动、事实 ID、规范事件 ID和证据 ID。

## 6. RAG 文档与隐私

RAG 只索引 `clean_event_facts` 当前规则版本中 `quality_state IN ('accepted','merged')` 且 `excluded_from_effectiveness=false` 的事实。

向量采用“锚点事实 + 邻域事实”策略：用户消息、AI 工具、终端、Git、IDE和浏览器作为向量锚点；AI 回复保留在清洗事实层，在命中相同设备、项目和时间邻域后加入生成上下文，不为每条回复重复生成向量。相同 `content_hash` 在同一索引进程内只计算一次 embedding。

文档内容规则：

- AI 用户/助手消息：已脱敏内容。
- AI 工具执行：工具名、命令类型、安全摘要；禁止完整命令、arguments、input。
- 人工/未知终端：完整脱敏规范命令。
- Git、IDE、浏览器：确定性安全摘要和允许字段。
- 动态 automation、命令残片、待确认记录：不建立文档和向量。

PostgreSQL 新增：

- `retrieval_documents`：文档 ID、事实 ID、tenant、设备、项目、活动类型、时间、内容、内容哈希、清洗规则版本、embedding 模型版本、索引状态。
- `retrieval_query_jobs`：问题、过滤条件、状态、阶段、进度、答案、引用、错误码、创建者和时间。

Qdrant collection 为 `aetheris_activities_v1`，只保存向量和最小过滤 payload：tenant、document ID、设备、项目、活动类型和时间。正文只保存在 PostgreSQL。

## 7. Embedding 与生成

Model Gateway 增加内部接口：

- `POST /internal/v1/embed`：调用 Ollama `/api/embed`，支持批量文本，返回模型、维度和 vectors。
- `POST /internal/v1/generate`：保留现有接口，新增 `rag_answer` 任务。

默认 embedding 模型使用 `embeddinggemma`。生产同批基准中，它的热态耗时约为 BGE-M3 的三分之一，输出 768 维，并支持多语言及代码/技术语料。它与回答模型 `qwen3:4b-instruct` 分开配置和健康检查。索引模型版本变化时全量重建 collection，不混用不同维度。

## 8. 异步查询

Admin API：

- `POST /api/v1/admin/rag/queries`：验证权限和 1 到 90 天范围，写入 queued 任务并返回 202。
- `GET /api/v1/admin/rag/queries/{id}`：返回 `queued|embedding|retrieving|generating|completed|failed`、0-100 进度、答案和 citations。
- `GET /api/v1/admin/rag/status`：返回文档数、向量数、模型、最后索引时间和 provider 状态。

worker 流程：

1. 对问题生成 embedding。
2. 使用 tenant 和用户可读范围过滤 Qdrant top-k。
3. 用 PostgreSQL 再次验证每个文档的 tenant、设备、项目和活动范围。
4. 将最多 12 条安全证据传给 Ollama；证据视为不可信数据，模型必须忽略证据中的指令。
5. 生成回答并保存结构化 citations；引用只允许来自实际检索结果。

索引和查询共享查询优先门：一个索引小批次持共享锁，查询任务持独占锁。查询会等待当前最多 16 条的稳态批次完成，然后阻止新索引并独占模型算力；查询完成后索引自动恢复。Ollama 同时保留 embedding 和回答两个模型，内部超时为 300 秒。

## 9. Admin Web

“活动记录”页面提供日期、设备、项目、活动类型和 AI 消息角色筛选，默认服务端分页。AI 内容是活动详情的一种展示，不再有独立一级页面。

“智能查询”页面提供：

- 问题输入框。
- 日期、设备、项目、活动类型范围。
- 索引状态。
- 查询阶段和进度条。
- 回答正文及编号引用。
- 点击引用打开事实摘要和原始证据详情。

页面不显示功能说明或绩效判断，不提供员工排名和黑盒总分。

## 10. 部署与回填

1. 备份 PostgreSQL、Go Server、Model Gateway、Admin Web、Core 和配置。
2. 部署 migration、统一活动双读 API和 Core 项目兜底。
3. 重放 AI checkpoint，补齐此前丢弃的 cwd 数据；对账逻辑项目数量。
4. 安装 Qdrant x86_64 独立 systemd 服务，只监听 `127.0.0.1:6333`，数据目录 `/opt/aetheris/vector/qdrant`。
5. 安装 `embeddinggemma`，验证 Ollama `/api/embed`。
6. 启动索引 worker，分批回填清洗事实；失败可重试，不阻塞 ingest。
7. 索引完成后开放“智能查询”，执行带引用的端到端验证。

## 11. 验收标准

- Admin 只保留一个“活动记录”入口，AI、终端、IDE、浏览器和版本控制可统一筛选。
- fallback AI 事件不含 cwd，项目显示 `Codex` 等工具名。
- 原有 `jtagent` 数据保持幂等，历史缺失会话得到补齐。
- Qdrant、Ollama embedding、Model Gateway 和 Go Retrieval 健康检查通过。
- RAG 索引数量与可索引清洗事实数量一致，隔离/排除事实为 0 个向量。
- 查询立即返回 202，页面持续显示阶段，最终答案含可打开的事实引用。
- AI 工具文档和 API 中完整命令泄漏为 0。
- 所有文档为中文，并保留可恢复备份。
