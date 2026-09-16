# Aetheris 过程知识增强智能查询设计

**日期：** 2026-09-16  
**状态：** 已确认，待实施  
**范围：** Windows Core、Go Server、PostgreSQL、Qdrant、Model Gateway、Admin Web  
**关联能力：** 统一活动、项目归属、清洗事实、智能查询、Work Episode、远期模型训练

## 1. 背景

Aetheris 的核心价值不是简单保存终端事件，也不是把当前项目实现状态交给模型复述，而是捕获研发人员与 AI 协作的完整过程，将问题、约束、探索、决策、失败、验证和最终结论转化为可复用知识。

当前智能查询直接索引清洗事实中的用户消息，并在查询命中后按设备、原始项目和时间窗口临时附加少量 AI 回复。这种方式在真实数据中出现以下问题：

- 用户提出的问题和检查项被模型误当成已经验证的技术事实。
- AI 的最终长回答没有独立建立索引，详细结论无法直接召回。
- AI 回复关联没有使用 `session_id`，历史会话时间集中时会关联到其他会话。
- 长消息正文最多保留 4,000 字，Embedding 只读取前 256 字，后半段知识不可检索。
- 活动页面使用逻辑项目 ID，RAG 文档和 Qdrant 使用原始项目 ID，项目筛选不一致。
- supersession 后的新事件可能丢失旧事件已经确认的逻辑项目归属。
- 单路向量检索缺少精确关键词、去重、主题覆盖和证据质量排序。
- 分析问题仍受固定 256 token 生成限制，无法形成完整结构。

生产回归问题“帮我分析，Windows 操作系统如何实现一个 EDR”暴露了上述缺陷。系统引用了多条要求分析 MiniFilter、WFP、通信和策略的用户提示词，却没有使用同一会话中已经存在的 7,000 至 12,000 字 AI 技术结论，最终把“需要检查什么”错误改写成“代码已经存在什么问题”。

## 2. 产品目标

本设计采用：

> 过程知识层 + 通用模型补全 + 已验证结果纠错

目标如下：

1. 从一个逻辑项目的完整人机交互过程中提取可复用知识。
2. 正确区分人工问题、人工约束、AI 探索、AI 最终结论和验证事实。
3. 使用通用模型知识补足本地过程未覆盖的标准概念和完整架构。
4. 已验证本地结果优先于通用知识，并明确说明其环境和适用条件。
5. 不把智能查询默认输出变成“当前项目能力清单”。
6. 每条本地过程结论都能追溯到知识单元、会话和原始事实。
7. 保持原始事件不可变，所有知识均可版本化重建、撤回和回滚。
8. 为远期监督微调、LoRA 和偏好训练形成受控的高质量数据基础。

## 3. 非目标与边界

- 首期不直接使用全部原始聊天训练模型。
- 不进行在线自动学习，不因一次新对话立即修改模型权重。
- 不把用户问题、AI 中间探索或被否定方案直接当作事实。
- 不让模型自行生成不可解释的知识可信度总分。
- 不覆盖或删除 `events`、`clean_event_facts` 和历史项目归属。
- 不把未授权项目、待确认命令残片或被策略排除数据放入知识索引。
- 不上传源代码正文、完整 AI 命令、未脱敏路径、凭据或高风险 OCR 内容。
- 不因为使用通用模型知识而绕过租户、主体、设备、项目和日期权限。
- 不要求所有查询都输出项目实现状态；只有用户明确询问项目现状时才进入能力盘点模式。

## 4. 已确认原则

### 4.1 知识边界

项目过程数据是回答的重要知识来源，但不是唯一知识来源。通用模型负责标准知识和缺口补全，过程知识负责提供真实研发经验、约束、反例和验证结果。

### 4.2 冲突优先级

已确认规则：

> 已验证本地结果优先，同时说明适用条件，不使用通用知识直接覆盖。

知识优先顺序为：

1. 已完成并验证的结果。
2. 人工确认的设计决策。
3. AI 最终回答。
4. 人工问题、追问和约束。
5. AI 中间分析过程。
6. 通用模型知识。

上述顺序用于结论选择，不用于生成员工、项目或模型的黑盒评分。

### 4.3 答案组织

答案按用户问题组织，不按当前项目模块组织。

例如用户询问 Windows EDR 时，应先解释 EDR 的采集、分析和响应架构，再把本地过程中验证过的 MiniFilter、WFP、IRQL、事件队列等经验自然融入对应章节。除非用户询问“项目目前实现了什么”，否则不得输出 `safe` 当前能力清单。

## 5. 总体架构

```text
events / clean_event_facts（不可变证据）
                 |
                 v
Process Session Assembler
  - 精确 session 关联
  - supersession 与逻辑项目继承
                 |
                 v
Process Turn Classifier
  - 人工问题 / 约束 / 确认 / 否定
  - AI 探索 / 最终结论
  - 工具结果 / 验证结果
                 |
                 v
Process Knowledge Extractor
  - 问题、结论、依据、备选、适用条件
  - 决策状态、验证状态、证据链
                 |
                 v
process_knowledge_units + chunks（PostgreSQL）
                 |
                 +--> Qdrant 稠密向量
                 +--> PostgreSQL 关键词索引
                 |
                 v
Hybrid Retriever + Evidence Planner
                 |
                 v
通用模型知识 + 过程知识 + 已验证纠错
                 |
                 v
结构化答案 + 逐结论引用 + 可展开证据
```

过程知识层是清洗事实和智能查询之间的独立边界。它不改变 Core 事件合同，也不要求 Model Gateway 连接数据库。

## 6. 会话重建

### 6.1 精确关联顺序

消息按以下顺序关联：

1. `tenant_id + device_id + logical_project_id + ai_tool + session_id`。
2. `conversation_id`、`message_id`、`parent_message_id` 等结构化关系。
3. 缺少会话 ID 时，使用项目、设备、来源、采集顺序和有界时间窗口建立低置信度会话。
4. 无法可靠关联的消息保留为孤立过程证据，不强行拼接。

首期禁止仅使用“同设备 + 同原始项目 + 时间窗口”关联回复。

### 6.2 逻辑项目

过程会话和知识单元统一使用 `logical_project_id`：

- 已注册项目使用当前激活项目归属版本。
- supersession 新事件默认继承被替代事件的逻辑项目归属。
- 新证据明确指向其他项目时，创建新的归属版本，不静默覆盖旧版本。
- `Codex`、`Claude Code` 等工具兜底项目只能作为临时未归属状态。
- 未归属会话可以生成候选知识，但不能进入某个已确认项目的知识索引。

### 6.3 会话生命周期

会话状态：

- `open`：仍可能收到新消息。
- `idle`：在配置时间内没有新消息。
- `closed`：存在明确结束标记或达到关闭条件。
- `stale`：源证据、项目归属或清洗版本发生变化。
- `superseded`：已经产生更新 revision。

新消息只标记会话为 `dirty`，不得在 ingest 请求内同步调用模型。

## 7. 过程轮次分类

### 7.1 分类类型

`process_turns.statement_kind` 支持：

- `human_question`
- `human_constraint`
- `human_followup`
- `human_confirmation`
- `human_rejection`
- `ai_exploration`
- `ai_final_answer`
- `tool_call`
- `tool_result`
- `code_change`
- `test_result`
- `build_result`
- `runtime_validation`
- `system_context`

### 7.2 基本规则

- 用户问题只表示意图，不得独立支持技术结论。
- 用户确认可以支持设计决策，但不能替代运行验证。
- AI 探索过程可以解释推理路径，不能覆盖 AI 最终回答。
- “执行了测试命令”与“测试通过”是不同状态。
- 工具输出、退出码、测试报告和明确用户确认可以形成验证证据。
- 被后续消息纠正的 AI 结论必须标记为 `contradicted` 或 `superseded`。

## 8. 数据模型

新增 migration，建议名称为 `020_process_knowledge.sql`。

### 8.1 `process_sessions`

| 字段 | 说明 |
|---|---|
| `tenant_id`、`session_id` | 租户与稳定会话标识 |
| `subject_id`、`device_id` | 主体与设备 |
| `logical_project_id` | 当前逻辑项目 |
| `ai_tool` | Codex、Claude Code、Cursor 等 |
| `source_session_id` | 原始工具会话标识 |
| `association_method` | exact、parent_chain、inferred_time 等 |
| `association_confidence` | high、medium、low |
| `state` | open、idle、closed、stale、superseded |
| `started_at`、`ended_at` | 会话范围 |
| `source_event_ids` | 原始证据 ID |
| `cleaning_rule_version`、`attribution_rule_version` | 来源版本 |
| `revision`、`supersedes_revision` | 知识重算版本 |

### 8.2 `process_turns`

| 字段 | 说明 |
|---|---|
| `turn_id`、`session_id`、`sequence` | 轮次与顺序 |
| `message_role` | user、assistant、system、tool |
| `statement_kind` | 问题、约束、最终结论、验证等 |
| `safe_content` | 已脱敏内容 |
| `content_hash` | 内容哈希 |
| `source_event_ids` | 来源事实 |
| `classification_method`、`classification_version` | 分类方式和版本 |
| `occurred_at` | 发生时间 |

### 8.3 `process_knowledge_units`

| 字段 | 说明 |
|---|---|
| `knowledge_id`、`revision` | 稳定知识 ID和版本 |
| `tenant_id`、`logical_project_id`、`session_id` | 权限与来源范围 |
| `topic`、`knowledge_type` | 主题和知识类型 |
| `problem` | 需要解决的问题 |
| `intent` | 人工真实目标 |
| `constraints` | 限制、反例和不可接受方案 |
| `conclusion` | 可复用技术结论 |
| `rationale` | 推理与选择原因 |
| `alternatives` | 比较过或被放弃的方案 |
| `applicability` | 操作系统、版本、组件和环境条件 |
| `caveats` | 已知边界和风险 |
| `decision_state` | proposed、accepted、rejected、superseded |
| `validation_state` | unverified、partially_verified、verified、contradicted |
| `lifecycle_state` | candidate、active、stale、withdrawn |
| `extractor`、`extractor_version` | 确定性规则或模型版本 |
| `created_at`、`updated_at` | 物化时间 |

### 8.4 `process_knowledge_evidence`

保存：

- `knowledge_id` 和 revision。
- `evidence_kind`：question、constraint、answer、tool、code、test、build、runtime、confirmation、rejection。
- `event_id`、`fact_id`、`turn_id`。
- `supports_section`：problem、conclusion、applicability、caveat 等。
- `relation`：supports、contradicts、supersedes、context。
- `reason_code`。

每个 `accepted` 或 `verified` 结论必须至少有一条有效证据。

### 8.5 `process_knowledge_chunks`

保存知识分块正文、块序号、内容哈希、Embedding 模型、关键词索引字段、向量键和索引状态。知识正文只在 PostgreSQL 保存，Qdrant 仍只保存向量与最小过滤 payload。

### 8.6 `process_knowledge_jobs`

保存会话范围、项目范围、知识版本、状态、游标、处理数、候选数、验证数、冲突数、失败码和开始/完成时间。

所有表包含 `tenant_id`、启用 RLS，并建立逻辑项目、会话、验证状态、主题、时间和证据 ID索引。

## 9. 长文本与知识分块

### 9.1 分段原则

先按语义结构切分会话，再切分长度：

- 一个问题和连续追问。
- AI 探索与工具调用。
- AI 最终结论。
- 人工确认、否定或新增约束。
- 代码修改、测试、构建和运行结果。

### 9.2 分块规则

- 每块约 600 至 1,000 个中文字符。
- 保留小范围重叠，避免语义边界断裂。
- Markdown 标题、列表、表格和代码标识尽量保持完整。
- Windows API、函数名、错误码和路径末级名称不得在中间截断。
- Embedding 输入使用完整知识块，不再固定截取前 256 字。
- 超长 AI 最终回答拆为多个同一知识单元的主题块。

### 9.3 双索引

- 过程知识索引：智能查询的主要来源。
- 原始活动索引：引用、复核和必要上下文补充。

原始活动索引不再直接承担复杂知识问答。

## 10. 混合检索

### 10.1 检索通道

并行执行：

1. Qdrant 稠密向量，处理语义同义表达。
2. PostgreSQL `pg_trgm` 或关键词索引，处理 Windows API、函数名、错误码和精确术语。

两路结果使用确定性 Reciprocal Rank Fusion 合并。

### 10.2 查询拆解

分析类问题先生成结构化子主题。EDR 回归问题至少拆为：

- 总体架构
- 内核采集
- 用户态代理
- 文件、进程和网络传感器
- 检测与关联
- 响应执行
- 性能与稳定性
- 已验证经验

每个子主题独立召回，避免同一提示词的重复文档占满上下文。

### 10.3 排序规则

显式规则：

```text
verified > partially_verified > unverified
accepted > proposed
AI 最终结论 > 人工问题 > AI 中间探索
精确术语命中提高对应子主题优先级
同一知识单元去重
同一会话限制数量
不同子主题保证最低覆盖
contradicted 只作为反例和边界
```

界面展示命中原因，不生成不可解释的单一可信度分数。

### 10.4 项目范围

过程知识查询默认选择一个逻辑项目。跨项目查询必须显式选择“全部项目”，并按项目设置召回配额。

Qdrant payload 至少包含：

- `tenant_id`
- `logical_project_id`
- `knowledge_id`
- `chunk_id`
- `topic`
- `knowledge_type`
- `decision_state`
- `validation_state`
- `occurred_at`

## 11. 上下文组装

模型接收的上下文按知识单元组织：

```text
问题定义
+ 人工约束和追问
+ AI 最终结论
+ 人工确认或否定
+ 验证结果
+ 适用条件
+ 必要原始证据引用
```

禁止把六条相似用户提示词直接作为六条事实交给生成模型。

过程知识与通用知识使用不同区段和来源标记。模型必须知道哪些内容是：

- 已验证本地经验。
- 人工确认设计。
- 未验证过程结论。
- 通用知识补充。
- 已否定反例。

## 12. 答案生成契约

### 12.1 两阶段生成

第一阶段生成回答计划：

```json
{
  "question_intent": "分析 Windows EDR 的实现方法",
  "required_topics": ["总体架构", "采集", "检测", "响应", "稳定性"],
  "claims": [
    {
      "claim": "内核回调只负责快速采集，复杂分析转移到用户态",
      "source_kind": "verified_process_knowledge",
      "knowledge_unit_ids": ["knowledge-..."],
      "applicability": "Windows 内核驱动高 IRQL 路径"
    }
  ],
  "coverage_gaps": ["威胁情报"]
}
```

第二阶段根据已验证回答计划生成自然语言。

### 12.2 冲突处理

| 场景 | 行为 |
|---|---|
| 已验证本地结果与通用知识不同 | 本地结果优先，并说明环境和适用条件 |
| 人工确认但未验证的方案与通用知识不同 | 并列说明，标记为设计选择 |
| AI 结论与验证结果冲突 | 验证结果优先，AI 结论标记为已否定 |
| 两条已验证结果冲突 | 保留版本或环境差异，不强行合并 |
| 只有人工问题 | 只能作为意图，不能形成事实 |
| 本地过程未覆盖 | 使用通用知识并明确其来源类型 |

### 12.3 置信度

服务端根据证据覆盖产生离散等级：

- `high`：关键结论有已验证知识和完整证据链。
- `medium`：主要由人工确认或部分验证知识支持，并有通用知识补充。
- `low`：只有未验证讨论、弱相关证据或通用模型知识。

模型不能自行提高置信度。

### 12.4 引用

本地过程结论必须引用 `knowledge_unit_id`，并可以展开到：

```text
知识单元
  -> 人工问题与约束
  -> AI 最终回答
  -> 工具与代码证据
  -> 测试、构建或用户确认
  -> 原始活动/事实 ID
```

默认页面展示安全摘要，不一次返回完整长会话。

### 12.5 输出预算

- 直接确认：约 256 token。
- 原因或步骤：约 512 至 800 token。
- 详细分析：约 1,500 至 3,000 token。
- 大型架构：允许分章节生成，再统一校验。

小模型负责分类、抽取和结构校验；复杂分析可以路由到更强的本地模型或 Dify provider，但证据和权限仍由 Go Server 控制。

### 12.6 生成后校验

服务端验证：

- 本地事实是否具有知识单元引用。
- 引用状态是否允许支持该结论。
- 是否把用户问题写成事实。
- 是否遗漏已验证冲突。
- 是否超出项目、设备和日期权限。
- 是否把过程经验错误表述成项目能力。
- 是否生成不存在的文件、函数或验证结果。

失败时允许一次结构化修正；再次失败时使用已验证知识生成确定性保底答案。

## 13. Model Gateway 任务

新增或扩展内部任务：

- `classify_process_turns`
- `extract_process_knowledge`
- `plan_rag_answer`
- `rag_answer`
- `verify_rag_answer`

Model Gateway 继续不连接 PostgreSQL。Go Server 提供最小、脱敏且已授权的会话片段或知识单元，并验证所有输出。

## 14. API 与 Admin Web

### 14.1 查询接口

现有异步查询接口保持兼容，扩展请求：

```json
{
  "question": "帮我分析 Windows 操作系统如何实现一个 EDR",
  "filters": {
    "from": "2026-08-18",
    "to": "2026-09-16",
    "logical_project_id": "logical-project-...",
    "knowledge_scope": "project_process"
  }
}
```

响应引用增加：

- `knowledge_id`
- `chunk_id`
- `source_kind`
- `decision_state`
- `validation_state`
- `applicability`
- `source_event_ids`

### 14.2 管理接口

- `GET /api/v1/admin/process-knowledge/summary`
- `GET /api/v1/admin/process-knowledge/units`
- `GET /api/v1/admin/process-knowledge/units/{id}`
- `POST /api/v1/admin/process-knowledge/backfills`
- `GET /api/v1/admin/process-knowledge/jobs/{id}`
- `POST /api/v1/admin/process-knowledge/versions/{version}/activate`
- `POST /api/v1/admin/process-knowledge/versions/{version}/rollback`

### 14.3 页面

智能查询页：

- 默认要求选择逻辑项目。
- “全部项目”是显式选择。
- 显示回答覆盖主题、总体置信度和本地/通用来源构成。
- 引用卡片显示人工确认、已验证、部分验证、通用补充或反例。
- 可以展开同一会话的必要片段。

新增“过程知识”诊断页：

- 会话数量、候选知识、已验证、冲突和未归属数量。
- 按项目、主题、状态和知识版本筛选。
- 查看知识单元证据链。
- 发起 dry-run、回填、激活和回滚。

## 15. 历史回填

### 15.1 顺序

1. 修复 supersession 逻辑项目继承。
2. 对所有 AI 工具按 `session_id` 重建会话。
3. 分类人类问题、约束、确认和 AI 最终回答。
4. 关联工具结果、代码修改、测试、构建和运行验证。
5. 生成候选知识单元和证据链。
6. 生成知识分块并写入新索引。
7. 对账并抽样复核。
8. 进入 shadow 模式。

### 15.2 任务模式

- `dry-run`
- `apply`
- `resume`
- `activate`
- `rollback`

任务按会话批次处理，保存持久游标。单个会话失败不能阻塞其他会话。

### 15.3 版本

知识抽取规则、模型、清洗规则、项目归属规则和分块规则都进入知识版本。重算写新 revision，不覆盖旧知识。

## 16. 增量处理

会话满足以下条件之一时提交知识提取任务：

- 收到明确结束标记。
- 连续配置时间没有新消息。
- 出现人工确认或否定。
- 出现测试、构建或运行验证。
- 管理员触发重算。
- 源事件被替代、删除或重新归属。

开放会话可生成 `provisional` 知识，但不能自动变为 `verified`。

## 17. 灰度与回滚

新索引使用独立集合，例如：

```text
aetheris_process_knowledge_v1
```

切换状态：

```text
shadow -> canary -> active
```

- shadow：新旧检索同时执行，只保存对比结果。
- canary：部分查询返回新答案。
- active：过程知识成为默认。
- 回滚：恢复旧查询实现和上一知识版本。

现有 `aetheris_activities_v1` 在验收完成前保留，不进行破坏性迁移。

## 18. 分阶段实施

### 阶段一：正确性修复

- supersession 继承逻辑项目。
- 活动与 RAG 统一使用逻辑项目 ID。
- 使用 `session_id` 关联问答。
- 区分用户问题和 AI 结论。
- 分析类回答使用动态 token 预算。

### 阶段二：过程知识层

- 数据库表、RLS 和索引。
- 会话重建器和轮次分类器。
- 知识提取器和证据链。
- 长文本分块与版本化回填。

### 阶段三：检索和回答

- 向量与关键词双路召回。
- 查询拆解、去重、主题覆盖和排序。
- 两阶段生成、冲突处理和逐结论引用。
- Admin Web 查询与诊断页面。

### 阶段四：评测与切换

- 固定问题集和影子对比。
- 人工抽样复核。
- canary 与 active 切换。
- 生产回滚演练。

## 19. 远期训练与微调

### 19.1 数据来源

训练候选只来自：

```text
decision_state=accepted
+ validation_state=verified
+ lifecycle_state=active
```

原始聊天、AI 中间探索、被否定方案和未验证结论不能直接进入正向训练语料。

### 19.2 数据集结构

每个训练样本保存：

- 问题和真实意图。
- 必要过程上下文。
- 人工确认约束。
- 经验证理想结论。
- 适用条件和边界。
- 被否定方案作为负例。
- 知识单元、证据和数据集版本。
- 撤回和重建标识。

### 19.3 实施顺序

1. 建立过程知识评测集。
2. 优化提示词与 RAG，形成稳定基线。
3. 对已验证样本进行监督微调或 LoRA。
4. 使用人工选择和被否定方案构造偏好数据。
5. 微调模型继续经过 RAG、引用和验证层，不允许绕过证据系统。

### 19.4 治理

- 训练数据集按版本冻结。
- 生成前检查租户授权和数据用途许可。
- 事件删除、知识撤回或验证状态变化时定位受影响样本。
- 不进行无审查的线上持续学习。
- 训练模型记录基础模型、参数、数据集版本和评测结果。

## 20. 安全与隐私

- 所有过程知识使用已脱敏内容。
- 绝对路径只用于本地或服务端受控归属，不进入普通知识正文。
- AI 命令仍只保存类型、安全摘要和设备内 HMAC。
- Qdrant 不保存知识正文和原始消息。
- 用户只能查询其授权逻辑项目的知识。
- 引用展开重新执行权限检查，不能依赖查询时缓存权限。
- 模型输入和输出不写普通日志。
- 知识提取失败只记录任务、会话、版本、错误码和计数。

## 21. 故障处理

- 过程知识任务失败不阻塞事件 ingest、heartbeat 和活动页面。
- Model Gateway 不可用时保留会话 dirty 状态并退避重试。
- 项目归属不明确时不进入已确认项目索引。
- 知识抽取 JSON 无效时允许一次修正，仍失败则保持 candidate。
- 向量写入失败时 PostgreSQL 保留 pending/failed 状态。
- 新知识版本未完成对账时不得激活。
- 回滚只切换版本和查询模式，不删除原始事件或旧知识。

## 22. 测试策略

### 22.1 会话与项目

- 同一 `session_id` 的问题和回复正确关联。
- 不同会话即使时间相同也不能串联。
- 缺少 session ID 的推断关联明确标记低置信度。
- supersession 继承逻辑项目。
- 选择 `safe` 不返回未归属 Claude Code 或其他项目数据。

### 22.2 知识提取

- 用户问题不能单独生成技术事实。
- AI 最终回答与探索过程正确区分。
- 人工确认改变决策状态但不自动改变验证状态。
- 测试命令和测试通过正确区分。
- 后续否定使旧知识进入 rejected、contradicted 或 superseded。

### 22.3 长文本

- 55,000 字会话后半段的 EDR 内容可以检索。
- API 名称和错误码不会在分块中间截断。
- 同一知识单元多块不会重复占满全部候选。

### 22.4 检索

- 语义同义问题由向量通道命中。
- `MiniFilter`、`WFP`、`PsSetCreateProcessNotifyRoutineEx` 由关键词通道命中。
- 子主题具有最低覆盖。
- 已验证结论优先于未验证讨论。
- 被否定方案只作为反例。

### 22.5 生成

- 本地结论有知识单元引用。
- 通用知识不冒充本地验证。
- 本地与通用冲突时本地验证优先并说明条件。
- 默认答案不输出项目能力清单。
- 分析问题不受 256 token 限制。
- 无效引用、虚构文件和无证据实现状态被拒绝。

## 23. 固定生产验收

问题：

> 帮我分析，Windows 操作系统如何实现一个 EDR

验收要求：

1. 覆盖总体架构、内核采集、用户态代理、检测、响应和管理闭环。
2. 使用 `safe` 研发过程中形成的人机协作经验。
3. 不把用户提示词当成事实。
4. 不输出 `safe` 当前能力清单。
5. 已验证经验与通用知识冲突时，本地结果优先并说明适用条件。
6. 每个本地经验结论可追溯到知识单元和原始证据。
7. 长回答结构完整，不受固定 256 token 限制。
8. 选择 `safe` 时不混入未归属 Claude Code 或其他项目会话。
9. 证据不足的子主题明确标记为通用知识补充。
10. 新旧检索影子对比证明相关性、完整性和事实正确性均有提升。

## 24. 完成标准

- 过程知识表、RLS、索引和任务可独立运行。
- 历史会话可 dry-run、续跑、激活和回滚。
- `safe` 项目历史数据完成项目归属、会话重建和知识提取。
- 新索引 pending/failed 为可解释状态，数量可对账。
- 智能查询完成两阶段生成和逐结论引用。
- Admin Web 可以查看知识来源和证据链。
- 固定 EDR 回归用例通过人工与自动化验收。
- 旧活动 RAG 保留到新方案 active 后的稳定观察期结束。
- 远期训练数据集只使用已接受且已验证知识，并具有版本、撤回和重建能力。

