# Aetheris 事件清洗与事实层实施计划

> **面向代理执行者：** 必须使用 `superpowers:executing-plans` 按任务执行，每项使用复选框跟踪。

**目标：** 建立原始事件不可变、清洗事实可版本重算的完整链路，正确合并 PowerShell 多行命令、标记 AI 工具执行、去重效能计数并提供数据质量视图。

**架构：** Windows Core 负责本地 PowerShell 逻辑命令组装、Codex/Claude 结构化 tool call 提取和设备内 HMAC；Go Server 负责把原始事件物化为版本化 `clean_event_facts`。Admin Web 只查询事实和证据链，效能聚合在回填对账后切换到当前规则版本。

**技术栈：** Python 3.12/PyInstaller、Windows PowerShell AST、DPAPI、Go 1.22/pgx/PostgreSQL 14、React/TypeScript/Vite、systemd。

**设计文档：** `docs/superpowers/specs/2026-09-07-event-cleaning-facts-design.md`

## 全局约束

- 不更新或删除 `events` 原始行；历史错误只通过新事实标记和排除。
- AI 命令不得进入事件 JSON、日志、API 响应或 Admin Web；只保存类型、80 字内摘要和设备内 HMAC。
- 人工/未知终端命令允许展示完整脱敏命令；归因为 AI 后只展示摘要。
- 残片、隔离和被合并的次级证据不进入效能计数。
- 日志不记录命令、消息、HMAC 或本机路径。
- 所有文档使用中文。
- 当前目录无 `.git`，不执行 Git 提交；每任务以测试通过和可恢复备份为交付关口。

---

### 任务 1：Core PowerShell 逻辑命令解析

**文件：**
- 创建：`src/aetheris/command_cleaning.py`
- 修改：`src/aetheris/adapters/terminal.py`
- 测试：`tests/test_command_cleaning.py`、`tests/test_terminal_adapter.py`

**接口：**
- 输入：PSReadLine 物理行和源偏移。
- 输出：`LogicalCommand(command, line_start, line_end, merge_method, quality_state, reason_codes)`。

- [x] 写入反引号三行合并、孤立参数倒序合并和无法合并残片的失败测试。
- [x] 运行 `python -m unittest tests.test_command_cleaning tests.test_terminal_adapter -v`，确认因逻辑命令组装缺失而失败。
- [x] 实现有界状态机和 PowerShell AST 完整性检查；AST 不可用时保守进入 `quarantined`。
- [x] 修改 `TerminalAdapter.collect()` 生成单条逻辑命令，保留行号/偏移和合并原因。
- [x] 运行定向测试；全量 Core 测试纳入发布关口。

### 任务 2：Core AI tool call 隐私提取

**文件：**
- 创建：`src/aetheris/command_privacy.py`
- 修改：`src/aetheris/adapters/ai_sessions.py`、`src/aetheris/tray.py`、`src/aetheris/setup_app.py`
- 测试：`tests/test_command_privacy.py`、`tests/test_ai_session_adapter.py`、`tests/test_tray.py`

**接口：**
- `CommandPrivacy.describe(command: str) -> CommandDescriptor`
- `CommandDescriptor(command_type, command_summary, command_hash)`
- AI 适配器输出 `event_type=ai.tool_call`，payload 含 tool/session/message/origin/time 和 descriptor。

- [x] 写入“相同密钥稳定、不同密钥不同、命令原文不进入 descriptor/event JSON”失败测试。
- [x] 写入 Codex `custom_tool_call/function_call` 和 Claude `tool_use` 失败测试。
- [x] 运行定向测试，确认因接口缺失而失败。
- [x] 实现 HMAC-SHA256、命令类型和确定性摘要；DPAPI 密钥接入在 Core 组装步骤完成。
- [x] 实现 Codex/Claude 结构化 tool call 解析，并在事件序列化测试中证明不存在命令原文。
- [x] 运行定向测试；全量 Core 测试纳入发布关口。

### 任务 3：PostgreSQL 事实表与 Go Cleaning Worker

**文件：**
- 创建：`server/migrations/007_clean_event_facts.sql`
- 创建：`server/internal/cleaning/model.go`、`rules.go`、`repository.go`、`service.go`、`worker.go`
- 修改：`server/cmd/aetheris-server/main.go`、`server/internal/config/config.go`
- 测试：`server/internal/cleaning/rules_test.go`、`service_test.go`、`migration_test.go`

**接口：**
- `NormalizeEvents([]RawEvidence, ruleVersion int) []Fact`
- `Service.RecomputeRange(ctx, tenantID, from, to, ruleVersion) (jobID string, error)`
- `Worker.Run(ctx)` 增量物化当前 tenant 的未处理证据。

- [x] 写入两类用户异常命令、AI/terminal HMAC 去重、残片排除和规则版本幂等失败测试。
- [x] 运行 `go test ./internal/cleaning -v`，确认因实现缺失而失败。
- [x] 实现 `clean_event_facts`、`cleaning_jobs`、RLS、多维和 GIN 索引。
- [x] 实现确定性规则、事实幂等 upsert、任务状态和有界后台 worker。
- [x] 运行 cleaning 测试、`go test ./...`和 `go vet ./...`。

### 任务 4：清洗 API 与 Admin Web

**文件：**
- 创建：`server/internal/httpapi/data_quality_handlers.go`
- 修改：`server/internal/httpapi/router.go`、`server/internal/aiinteractions/repository.go`、`contracts/openapi.yaml`
- 创建：`admin-web/src/pages/DataQualityPage.tsx`、`DataQualityPage.test.tsx`
- 修改：`admin-web/src/pages/AIInteractionsPage.tsx`、`admin-web/src/api/types.ts`、`client.ts`、`App.tsx`、`styles.css`

**接口：**
- `GET /api/v1/admin/data-quality/summary`
- `GET /api/v1/admin/data-quality/facts`
- `POST /api/v1/admin/data-quality/recompute`
- AI 交互结果增加 `interaction_kind`、`actor_origin`、`command_type`、`command_summary`、`quality_state`、`source_event_ids`。

- [x] 写入 API 权限、分页、原因码和重算 202 合同失败测试。
- [x] 写入 AI 工具执行无原命令入口、数据质量数量/证据链/重算状态的前端失败测试。
- [x] 运行 Go/React 定向测试，确认因端点和页面缺失而失败。
- [x] 实现独立 handler/repository 查询、AI 交互扩展和数据质量页面。
- [x] 运行 `go test ./...`、`go vet ./...`、`npm test -- --run`和 `npm run build`。

### 任务 5：个人效能事实切换

**文件：**
- 修改：`server/internal/effectiveness/repository.go`、`service.go`、`definitions.go`
- 测试：`server/internal/effectiveness/aggregate_test.go`、`service_test.go`

**接口：**
- `Repository.RecomputeDay` 只读当前 cleaning rule version 中 `excluded_from_effectiveness=false` 的规范事实。
- `MetricDefinitionVersion` 升级，报告显示清洗覆盖和规则版本。

- [x] 写入“AI/terminal 双证据只计一次、command_fragment 不计数、清洗未完成不混用新旧口径”失败测试。
- [x] 运行 effectiveness 定向测试确认失败。
- [x] 实现事实读取、新指标版本和覆盖提示。
- [x] 运行 effectiveness 和 Go 全量测试。

### 任务 6：发布、历史回填与验收

**文件：**
- 修改：`src/aetheris/version.py`、`pyproject.toml`、`deploy/.env.example`、`docs/operations/server-deployment.md`、`server-implementation-log.md`
- 产物：`releases/0.4.4/AetherisCore-0.4.4.exe`、`AetherisSetup-0.4.4.exe`

- [x] 运行 Core、Go、Model Gateway、Admin Web 和原生 provisioning 全量测试。
- [x] 构建 Core/Setup、Linux Go Server 和 Admin Web，生成 SHA-256。
- [x] 备份 PostgreSQL、服务端二进制/静态资源和本机 Core/队列/配置。
- [x] 先部署 migration/Go/Admin 双读版，再替换并启动 Core。
- [x] 分段回填 cleaning facts，对账 raw/fact/merged/fragment/quarantined 数量。
- [x] 验证用户提供的两组 PowerShell 样本各形成一条规范事实，AI 命令无原文泄漏。
- [x] 对账后切换效能口径并分段重算，验证页面/API/索引/服务状态。
- [x] 更新中文运维记录，保留回滚备份，清理可再生成的 staging 文件。
