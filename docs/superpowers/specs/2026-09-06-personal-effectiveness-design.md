# Aetheris 个人效能分析设计

**日期：** 2026-09-06  
**状态：** 待实现  
**范围：** Go Server、PostgreSQL 日聚合、Admin Web 个人效能页面、可选模型总结。

## 1. 目标与边界

个人效能模块把已经脱敏并持久化的工作事件转换成可解释的个人工作模式指标。它帮助用户和授权管理者理解投入分布、工作节奏、交付活动、工具使用和趋势变化，不用于自动评价员工。

本模块必须遵守以下边界：

- 不生成跨员工排名、排行榜或强制分位数。
- 不生成不可解释的综合总分。
- 不把事件数量直接称为生产力或绩效。
- 不把活跃时间桶称为工时、在线时长或加班时长。
- 所有指标必须给出计算口径、数据覆盖度和可下钻的事件证据。
- Ollama/Dify 只能生成文字总结，不能修改基础指标或产生隐藏评分。
- 只使用服务端已持久化的脱敏事件，不读取 Core 原始数据。

## 2. 方案选择

采用“日聚合表 + 当前日增量重算”方案。

相比每次请求扫描全部事件，该方案在数据增长后仍能稳定查询 7/30/90 天范围。相比直接让模型生成效能结论，该方案可审计、可重复计算，并且不会因模型版本变化破坏历史口径。

效能模块作为独立 Go package：

```text
events（不可变事实）
   |
   v
effectiveness Aggregator（确定性计算）
   |
   v
subject_effectiveness_daily（日聚合）
   |
   +--> Admin API --> Admin Web
   `--> 最小投影 --> Model Gateway（可选文字总结）
```

事件接收链路不等待聚合完成。后台任务每 5 分钟重算“今天”和“昨天”，修复晚到事件；管理员可以触发指定日期范围的受限重算。

## 3. 指标口径

所有时间计算先把 `occurred_at` 转换到租户配置时区。第一版由 `EFFECTIVENESS_TIMEZONE` 配置，默认且生产环境使用 `Asia/Shanghai`。API 可以回显时区但不能请求任意其他时区，避免为每个查询产生重复聚合。单次查询最大范围为 90 天。

### 3.1 活跃天数

指定周期内至少包含一个有效事件的自然日数量。

### 3.2 可解释活跃时段

把事件发生时间向下取整到 5 分钟边界；每个主体的每个不同 5 分钟桶记为 5 分钟。多个终端在同一时间桶内只能计一次。

该指标命名为 `active_window_minutes`，页面显示“活跃时段”，并明确注明“基于有事件的 5 分钟窗口，不等同于工时”。单日最大值为 1440 分钟。

### 3.3 工作会话

将主体的全部终端事件按时间排序。相邻事件间隔超过 30 分钟时开始新会话。会话的活跃分钟数等于该会话覆盖的不重复 5 分钟桶数量乘以 5；只有一个事件的会话记为 5 分钟。

输出：

- `session_count`：会话数量。
- `average_session_minutes`：会话活跃分钟数的算术平均值。
- `longest_session_minutes`：最长会话的活跃分钟数。

### 3.4 连续专注时段

在同一项目内，把相邻且间隔不超过 10 分钟的 5 分钟桶组成连续区间。区间达到 25 分钟且期间项目切换次数为 0，记为一个专注时段。

输出 `focus_block_count` 和 `focus_block_minutes`。该指标表示持续产生工作信号的项目专注窗口，不表示主观专注程度。

### 3.5 上下文切换

按主体时间顺序观察事件：相邻事件间隔不超过 30 分钟，且 `project_id` 不同时，记一次项目上下文切换。设备切换但项目不变不重复计数。

输出 `context_switch_count`。页面不把高或低切换数量直接解释为好坏。

### 3.6 工作活动分类

事件按确定性分类表统计：

- `delivery_events`：`git.commit`、`git.diff`。
- `coding_events`：`process.observed`、`ide.activity`、`vscode.activity`、`visualstudio.activity`。
- `terminal_events`：`terminal.command`。
- `ai_collaboration_events`：来源以 `core.ai.` 开头或事件类型为 `ai.session`、`ai.message`。
- `other_events`：不属于以上分类的合法事件。

分类表在 Go 代码中版本化为 `metric_definition_version`。同一事件只进入一个主分类，优先级为 delivery > AI collaboration > terminal > coding > other。

### 3.7 项目与工作角色分布

按 `project_id` 和事件入库时固化的 `work_role_code` 分组，输出事件数量、活跃时段和占比。角色后续变更不回写历史数据。

### 3.8 趋势

当前周期与紧邻的等长上一周期比较，输出绝对值、差值和百分比。上一周期为 0 时百分比为 `null`，不得显示无限增长。

趋势只描述变化，例如“活跃时段增加 20%”，不自动解释为效能提升或下降。

### 3.9 数据覆盖度

输出：

- `covered_days`：有事件的天数。
- `period_days`：查询周期自然日数。
- `coverage_ratio`：`covered_days / period_days`。
- `source_count`：不同事件来源数量。
- `device_count`：参与采集的不同终端数量。
- `last_event_at`：最后事件时间。

覆盖率低于 30% 时，页面显示“数据不足，不建议进行趋势判断”。这是数据质量提示，不是效能评分。

## 4. 数据库设计

新增 migration `005_personal_effectiveness.sql`。

### 4.1 `subject_effectiveness_daily`

```text
tenant_id                  TEXT
subject_id                 TEXT
local_date                 DATE
timezone                   TEXT
metric_definition_version  INTEGER
active_window_minutes      INTEGER
session_count              INTEGER
total_session_minutes      INTEGER
longest_session_minutes    INTEGER
focus_block_count          INTEGER
focus_block_minutes        INTEGER
context_switch_count       INTEGER
delivery_events            INTEGER
coding_events              INTEGER
terminal_events            INTEGER
ai_collaboration_events    INTEGER
other_events               INTEGER
project_breakdown          JSONB
work_role_breakdown        JSONB
source_counts              JSONB
device_ids                 JSONB
evidence_event_ids         JSONB
first_event_at             TIMESTAMPTZ
last_event_at              TIMESTAMPTZ
computed_at                TIMESTAMPTZ
```

主键为 `(tenant_id, subject_id, local_date, timezone, metric_definition_version)`。索引覆盖租户、主体和日期范围。启用与其他租户表相同的 RLS。

`evidence_event_ids` 每个指标类别最多保留 50 个代表性 ID；完整下钻通过带相同筛选条件的事件查询完成，避免日聚合行无限增长。

### 4.2 `effectiveness_recompute_jobs`

记录人工或后台重算的范围、状态、错误码、开始/结束时间和 definition version。它不保存事件 payload。

### 4.3 `user_subject_links`

把 Admin `user_id` 显式绑定到采集主体 `subject_id`。一个 Admin 用户在一个租户内最多绑定一个主体，用于执行 `member` 只能读取本人的权限规则。该表不能替代 `devices.subject_id`，也不能用于工作角色分配。

## 5. Go 模块边界

新增 `server/internal/effectiveness/`：

- `definitions.go`：版本化事件分类与时间阈值。
- `aggregate.go`：纯函数日聚合，输入按时间排序的最小事件投影。
- `repository.go`：读取事件、upsert 日聚合、读取周期结果。
- `service.go`：权限范围、当前/上一周期组合、趋势和覆盖度。
- `worker.go`：后台定时重算与失败隔离。

聚合器不依赖 HTTP、Admin Web 或 Model Gateway。HTTP handler 只负责参数校验、授权和序列化。

## 6. API

### 6.1 查询个人效能

```text
GET /api/v1/admin/effectiveness
  ?subject_id=subject-...
  &from=2026-08-08
  &to=2026-09-06
```

响应包含：主体信息、工作角色、周期、KPI、分类分布、项目分布、每日时间序列、当前/上一周期趋势、覆盖度、口径说明和证据事件 ID。

### 6.2 主体列表摘要

```text
GET /api/v1/admin/effectiveness/subjects?from=...&to=...
```

只返回授权范围内主体的活跃天数、活跃时段、事件分类和覆盖度，用于选择主体；不返回排名字段，也不提供按效能指标排序参数。

### 6.3 重算

```text
POST /api/v1/admin/effectiveness/recompute
```

仅 `tenant_admin`/`platform_admin` 或 `effectiveness:manage` 权限可调用；单次最大 31 天。请求进入后台 job，不阻塞 HTTP。

### 6.4 模型总结

```text
POST /api/v1/admin/effectiveness/summary
```

需要 `models:invoke`。Go Server 发送聚合指标、口径和证据 ID，不发送未筛选原始事件。响应必须标记 provider/model 和生成时间，并显示“AI 总结，不作为绩效评价”。

## 7. 权限

新增权限：

- `effectiveness:read`：读取授权范围内个人效能。
- `effectiveness:manage`：触发重算和管理口径版本。

授权规则：

- `platform_admin`、`tenant_admin`：读取所属租户全部主体。
- `analyst`、`reviewer`：第一版不授予个人效能读取权限，避免用不完整项目范围生成误导性的个人结论。
- `member`：只有其 Admin 用户通过 `user_subject_links` 显式关联 `subject_id` 时才能读取本人数据。
- `device_ingest`：不能读取任何效能数据。

API 不提供跨主体排名。主体列表默认按姓名或最近事件时间排序。

## 8. Admin Web

侧边导航新增“个人效能”。页面结构：

1. 主体选择、7/30/90 天、项目和时区筛选。
2. KPI：活跃天数、活跃时段、工作会话、专注时段、上下文切换、数据覆盖率。
3. 每日趋势图：活跃时段、会话、分类事件的时间序列。
4. 活动构成：交付、编码、终端、AI 协作、其他。
5. 项目与工作角色分布。
6. 指标口径和数据质量提示。
7. 证据下钻到现有事件详情。
8. 可选“生成 AI 总结”，默认不自动调用模型。

页面不显示总分、红黄绿绩效灯、排名、奖惩语言或未经口径支持的“效率提升”结论。

## 9. 故障与重算

- 聚合失败不影响事件 ingest。
- 某主体/日期失败时只标记对应 job，其他日期继续处理。
- 晚到事件通过重算今天和昨天进入聚合。
- definition version 变化时写新版本行，不覆盖旧版本，切换完成后再清理过期版本。
- API 查询无聚合数据时返回空状态和覆盖度 0，不伪造零效能结论。
- 时区无效、范围超过 90 天或 `from > to` 返回 400。

## 10. 测试与验收

1. 同一主体多终端的相同 5 分钟桶只计一次。
2. 30 分钟会话边界、10 分钟连续桶和 25 分钟专注阈值有边界测试。
3. 项目切换只在同一会话内计数。
4. 当前周期、上一周期和除零趋势行为确定。
5. 工作角色变更不改变旧聚合行。
6. 低覆盖率明确显示数据不足。
7. `member` 不能读取其他主体，`device_ingest` 永远被拒绝。
8. 页面能切换主体和周期，展示 KPI、趋势、构成、证据和空/错/加载状态。
9. API 和页面中不存在排名或综合分数字段。
10. Ollama/Dify 不可用时基础效能页面仍可完整使用。
