# Aetheris 事件清洗与事实层设计

**日期：** 2026-09-07  
**范围：** Windows Core、Go Server、Admin Web；原始事件继续使用现有 ingest 合同  
**状态：** 已实施并完成生产回填

## 1. 目标

在不修改、覆盖或删除原始事件的前提下，建立可版本化、可重算、可追溯的清洗事实层，解决以下问题：

- PowerShell 逻辑命令被 PSReadLine 拆成多个物理行和多条事件。
- Codex/Claude Code 结构化工具调用未被识别为 AI 执行的终端操作。
- 同一操作同时出现在 AI tool call 和终端历史时被重复统计。
- 无法独立执行的命令残片进入个人效能指标。
- Admin Web 无法说明“哪台设备、哪个项目、谁发起、哪些原始证据”。

## 2. 非目标与边界

- 不对员工排名，不生成绩效分、综合分或好坏判断。
- 不用命令文本猜测 AI 身份；只有结构化 AI tool call 或通过可验证关联的记录才标记为 `ai`。
- AI 执行命令不上传、不保存、不展示完整命令；只保留命令类型、确定性摘要和设备内有键哈希。
- 不修改既有 `events` 行；清洗规则升级通过新版本事实重算完成。
- 清洗故障不阻塞 heartbeat 和原始事件 ingest。

## 3. 总体架构

```text
PSReadLine / Codex JSONL / Claude JSONL
                |
                v
Windows Core 本地解析与脱敏
  - PowerShell 逻辑命令组装
  - AI tool call 结构化提取
  - 本机 HMAC 命令指纹
                |
                v
原始事件 events（不可变）
  terminal.command / ai.message / ai.tool_call
                |
                v
Go Cleaning Worker（版本化规则）
                |
                v
clean_event_facts + cleaning_jobs
       |                    |
       v                    v
个人效能聚合        Admin Web AI 交互 / 数据质量
```

Core 负责只能在本机可靠完成的解析和隐私处理；Go Server 负责跨原始事件的关联、规则版本、重算和查询。两者通过现有事件合同通信，不建立 Core 直连数据库的耦合。

## 4. Windows Core 采集与本地清洗

### 4.1 PowerShell 逻辑命令

Core 不再将 `ConsoleHost_history.txt` 的每个物理行直接转换为事件。解析流程如下：

1. 保留源文件偏移、物理行号和采集顺序。
2. 先按 PowerShell 尾随反引号的奇偶语义组装显式多行命令：只有奇数个尾随反引号表示续行，原因码为 `explicit_backtick_join`；偶数个反引号不抢占下一条记录。
3. 调用 `System.Management.Automation.Language.Parser.ParseInput` 验证候选逻辑命令是否语法完整。
4. 对以 PowerShell 参数开头、无法独立执行的行，尝试与相邻主命令组合；只有组合后 AST 完整时才合并，原因码为 `orphan_parameter_join`。
5. 对“参数行在前、主命令在后”的 PSReadLine 倒序样本，先按“主命令 + 参数片段”重排再做 AST 验证，并标记 `inferred_reordered_join`。
6. 仍不完整的物理行保留为原始证据，清洗事实标记 `command_fragment` 和 `excluded_from_effectiveness=true`。

用户提供的两类样本必须成为固定回归用例：

```powershell
Start-Process powershell.exe -ArgumentList `
  "-NoExit", "-Command", `
  "Set-Location ..."
```

```powershell
Get-ChildItem "D:\Program Files (x86)\Windows Kits\10\build" -Recurse -Filter Microsoft.DriverKit.Build.Tasks.17.0.dll
```

### 4.2 AI 结构化工具调用

Codex 和 Claude Code 适配器新增 `ai.tool_call` 输出。只解析官方本地历史中的结构化 tool call，不从 AI 自然语言回复中推断命令。Codex `custom_tool_call name=exec` 只静态提取 `tools.exec_command` 的静态 `cmd` 字符串，不执行编排代码；无法静态确定的调用标记为 automation 待确认。

`ai.tool_call` payload 仅包含：

```json
{
  "tool": "codex",
  "tool_call_type": "shell",
  "command_type": "file.search",
  "command_summary": "递归查找文件",
  "command_hash": "hmac-sha256:...",
  "session_id": "...",
  "message_id": "...",
  "actor_origin": "ai",
  "timestamp": "2026-09-07T10:00:00Z"
}
```

payload 不得包含命令原文、完整参数、完整路径、环境变量或 tool call arguments。

### 4.3 命令分类与摘要

首版使用确定性词法/AST 规则，不调用模型：

- `file.list`：列出目录或文件。
- `file.search`：查找文件或内容。
- `build`：编译、打包、生成。
- `test`：单元测试、集成测试、验收测试。
- `vcs`：Git/SVN 查询或变更。
- `process`：启动、结束、检查进程。
- `network`：HTTP、DNS、端口或网络连通性。
- `package`：依赖安装或包管理。
- `script`：通用脚本执行。
- `other`：无法归类。

`command_summary` 只由命令类型和安全选项生成，最长 80 个 Unicode 字符。例如 `Get-ChildItem ... -Recurse -Filter ...` 生成“递归查找文件”，不保留目录和文件名。

### 4.4 设备内有键指纹

Core 首次运行时生成随机 `command_fingerprint_key`，使用 Windows DPAPI CurrentUser 加密保存，不上传服务端。

```text
command_hash = HMAC-SHA256(device_local_key, canonical_redacted_command)
```

该指纹只用于同一设备上 AI tool call 与终端证据关联，不用于跨设备用户画像。使用随机有键 HMAC 而不是裸 SHA-256，防止通过常见命令字典反查命令。

## 5. 清洗事实数据模型

### 5.1 `clean_event_facts`

| 字段 | 类型 | 说明 |
|---|---|---|
| `fact_id` | TEXT | 基于 tenant、事实类型和源事件 ID 生成的稳定逻辑 ID，不包含规则版本 |
| `tenant_id` | TEXT | 租户边界 |
| `subject_id` | TEXT | 终端用户主体 |
| `device_id` | TEXT | 设备维度 |
| `project_id` | TEXT | 项目维度 |
| `occurred_at` | TIMESTAMPTZ | 规范事实时间 |
| `fact_type` | TEXT | `terminal_operation`、`ai_interaction`、`command_fragment` |
| `actor_origin` | TEXT | `human`、`ai`、`unknown` |
| `message_role` | TEXT | `user`、`assistant`、`system`、`tool`、`unknown` |
| `ai_tool` | TEXT | `codex`、`claude_code` 等 |
| `command_type` | TEXT | 确定性命令分类 |
| `command_summary` | TEXT | 不含参数/路径的脱敏摘要 |
| `command_hash` | TEXT | 设备内 HMAC 指纹 |
| `quality_state` | TEXT | `accepted`、`merged`、`quarantined` |
| `confidence` | TEXT | `high`、`medium`、`low` |
| `rule_version` | INTEGER | 清洗规则版本 |
| `reason_codes` | TEXT[] | 决策原因码 |
| `source_event_ids` | TEXT[] | 所有原始证据 ID |
| `canonical_event_id` | TEXT | 主证据 ID |
| `excluded_from_effectiveness` | BOOLEAN | 是否排除效能聚合 |
| `created_at` / `updated_at` | TIMESTAMPTZ | 物化时间 |

主键为 `(tenant_id, fact_id, rule_version)`，允许同一逻辑事实的多个规则版本并存。必须开启 tenant RLS，并建立 tenant/device/project/time、tenant/fact_type/quality_state 以及 GIN `source_event_ids` 索引。

### 5.2 `cleaning_jobs`

保存 `job_id`、tenant、时间范围、目标规则版本、状态、处理数、合并数、隔离数、失败原因和开始/完成时间。同一 tenant + rule version + range 只允许一个活动任务。

## 6. 执行者归因与去重

### 6.1 归因规则

- 结构化 `ai.tool_call`：`actor_origin=ai`、`confidence=high`。
- 与 `ai.tool_call` 命令 HMAC、设备和项目一致，且时间在 ±30 秒内的 `terminal.command`：归并为 AI 操作，`confidence=high`。
- 无历史时间或 cwd 的 PSReadLine 回填数据：只对同设备 HMAC 在 30 分钟接收窗口内做一对一归并，由 AI tool 提供权威项目并记录 `project_inferred_from_ai`，`confidence=medium`；同一终端证据不得被重复消费。
- 来自 PSReadLine 的新增互动记录且无 AI 匹配：`actor_origin=human`、`confidence=medium`。
- 无法确定执行者或时间：`actor_origin=unknown`。

### 6.2 去重规则

同一 AI tool call 与终端记录匹配时：

- 保留两条原始事件。
- 生成一条 `quality_state=merged` 的 `terminal_operation` 事实。
- `source_event_ids` 同时包含 AI tool call 和 terminal event ID。
- `canonical_event_id` 优先指向结构化 AI tool call。
- 个人效能只计算该事实一次。

## 7. Cleaning Worker

Go Server 增加独立 `cleaning` 模块和后台 worker：

1. 按 tenant + event ID 增量读取原始事件。
2. 为 `ai.message`、`ai.tool_call`、`terminal.command` 生成规范事实。
3. 在设备、项目、HMAC 和时间窗口内关联 AI/terminal 证据。
4. 幂等 upsert 指定 `rule_version`的事实。
5. 更新任务计数，不在日志中输出命令、消息或哈希。

重算不覆盖旧版本事实。新版本完成验收后再切换当前指标版本，便于对比和回滚。

## 8. API

### 8.1 AI 交互

`GET /api/v1/admin/ai-interactions` 增加：

- `interaction_kind=message|tool_call`
- `actor_origin=human|ai|unknown`
- 消息角色增加“AI 工具执行”展示类别。
- tool call 只返回 `command_type`、`command_summary`、`ai_tool`、时间和证据 ID，不返回完整命令。

### 8.2 数据质量

- `GET /api/v1/admin/data-quality/summary`：原始事件数、已清洗事实数、合并数、残片数、隔离数、当前规则版本和最后任务。
- `GET /api/v1/admin/data-quality/facts`：按设备、项目、原因码、质量状态、时间分页查询。
- `POST /api/v1/admin/data-quality/recompute`：只允许 `effectiveness:manage` 或后续独立 `cleaning:manage` 权限，返回 202 + `job_id`。

所有读取要求 `events:read`，写操作进入 `audit_logs`。

## 9. Admin Web

### 9.1 AI 交互页

保留现有“设备 → 项目 → 角色”三层布局，第三层展示：

- 用户发送
- AI 回复
- AI 工具执行
- 系统消息
- 未识别

AI 工具记录表格只展示命令类型和脱敏摘要，详情抽屉展示源事件 ID、规则版本、原因码和关联状态，不提供“查看完整命令”能力。

### 9.2 数据质量页

新增“数据质量”主导航：

- 顶部展示可解释数量，不生成黑盒质量分。
- 按原因码展示合并、残片、隔离和待处理数量。
- 支持查看事实到原始事件的证据链。
- 支持按日期范围发起重算任务并查看状态。

## 10. 个人效能切换

1. 清洗事实层完成全量回填前，现有指标继续读原始事件。
2. 完成数量对账和抽样后，通过 `metric_definition_version` 升级切换到清洗事实。
3. `command_fragment` 和 `excluded_from_effectiveness=true` 事实不进入活动窗口、会话、上下文切换或活动构成。
4. 一条 `merged` 事实无论有多少原始证据，只计算一次。
5. 页面持续显示口径版本和覆盖率，不将新旧版本直接混合比较。

## 11. 历史回填与发布顺序

1. 部署数据库表、RLS、索引和 Go Cleaning Worker，但暂不切换效能数据源。
2. 发布 Core 新版，新增 PowerShell 逻辑命令、Codex/Claude `ai.tool_call`、本机 HMAC 和原始行号/偏移。
3. 新事件增量生成清洗事实，验证 ingest 不受影响。
4. 使用本地 PSReadLine/Codex/Claude 历史重建有源偏移的补充事件，为现有历史数据生成事实。
5. 历史数据缺少可靠时间或存在多个同命令候选时，保持 `unknown/quarantined`，不强行合并。
6. 对账原始数、事实数、合并数、排除数和各原因码，人工抽样用户提供的异常命令。
7. 升级指标定义版本并分段重算个人效能。

任何阶段失败都可停止 cleaning worker 并回到原始事件指标，不需要恢复或修改 `events`。

## 12. 错误处理与可观测性

- 解析失败产生质量状态和原因码，不丢弃原始证据。
- worker 使用有界批次、指数退避和幂等 upsert。
- 监控原始事件积压、清洗延迟、隔离比例、版本覆盖率和失败任务。
- 日志只记录 tenant、job ID、rule version、数量、原因码和耗时；禁止记录命令、哈希、消息或本机路径。

## 13. 测试与验收

### Core

- 反引号三行命令合并为一条 AST 完整命令。
- 孤立 `-Recurse -Filter ...` 与相邻 `Get-ChildItem ...` 重排合并为一条命令。
- 无法合并的片段标记 `command_fragment`。
- Codex/Claude 结构化 tool call 生成 `ai.tool_call`，事件 JSON 中不存在命令原文或完整参数。
- 相同命令在同一设备上生成相同 HMAC，不同设备密钥产生不同 HMAC。

### Go Server

- migration 创建两张表、RLS 和必要索引。
- 增量重试不产生重复事实。
- AI/terminal 匹配后仅生成一条可计数事实并保留两个源 ID。
- 残片不进入效能聚合。
- 旧新规则版本可并存且可回滚。

### Admin Web

- AI 交互页展示 AI 工具执行，但不存在查看完整 AI 命令的入口。
- 数据质量页显示合并/残片/隔离数量、原因码、证据链和重算状态。
- 所有页面不展示排名、黑盒总分、命令原文或完整本机路径。

### 线上验收

- 备份 PostgreSQL 和当前 Core 本地数据。
- 先部署不切换指标的双读版本，再发布 Core。
- 验证两条用户已确认的多行命令各生成一条事实。
- 对账数量并抽样证据链后，才切换个人效能定义版本。

## 14. 已确认决策

- 采用“原始事件不可变 + 清洗事实层”。
- AI 命令只保存命令类型、确定性摘要和有键哈希。
- AI tool call 与终端证据同时保留，事实层去重计数。
- 命令残片不进入个人效能。
- Admin Web 增加 AI 工具执行和数据质量视图。
