# Aetheris 个人效能实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: 使用 `superpowers:executing-plans` 按任务逐项实现。步骤使用复选框跟踪。

**目标：** 在 Go Server、PostgreSQL 和 Admin Web 中交付可解释、可追溯且不含员工排名或黑盒总分的个人效能分析。

**架构：** 独立 `effectiveness` Go package 从脱敏事件计算日聚合，后台 worker 重算今天和昨天，HTTP API 组合周期趋势与覆盖率；Admin Web 只消费聚合 API，Ollama/Dify 只生成可选文字总结。

**技术栈：** Go 1.22+、pgx/v5、PostgreSQL 14+、React、TypeScript、Vite、Vitest。

**设计文档：** `docs/superpowers/specs/2026-09-06-personal-effectiveness-design.md`

## 全局约束

- 所有新增或修改文档使用中文。
- 不生成跨员工排名、排行榜、强制分位数或不可解释综合总分。
- 活跃时段使用不重复 5 分钟桶，不称为工时。
- 会话间隔阈值 30 分钟；专注时段阈值 25 分钟、连续桶间隔不超过 10 分钟。
- 默认时区为 `Asia/Shanghai`，单次查询最大 90 天，重算最大 31 天。
- 所有指标必须包含口径、覆盖率和事件证据 ID。
- 聚合或模型失败不能阻塞事件 ingest。
- 第一版仅 `platform_admin`、`tenant_admin` 和绑定主体的 `member` 可读取个人效能。

### Task 1：确定性聚合算法

**Files:**
- Create: `server/internal/effectiveness/definitions.go`
- Create: `server/internal/effectiveness/aggregate.go`
- Test: `server/internal/effectiveness/aggregate_test.go`

**Interfaces:**
- Produces: `AggregateDay(events []EventPoint, day time.Time, location *time.Location) DailyMetrics`
- Produces: `Classify(eventType, source string) ActivityClass`
- `DailyMetrics` 包含活跃桶、会话、专注时段、上下文切换、五类事件、项目/角色/来源分布和证据 ID。

- [ ] **Step 1: 写聚合边界失败测试**

```go
func TestAggregateDayDeduplicatesDevicesInSameBucket(t *testing.T) {
    events := []EventPoint{
        {EventID: "e1", DeviceID: "d1", ProjectID: "p1", OccurredAt: at("2026-09-06T01:01:00Z")},
        {EventID: "e2", DeviceID: "d2", ProjectID: "p1", OccurredAt: at("2026-09-06T01:04:00Z")},
    }
    got := AggregateDay(events, at("2026-09-06T00:00:00Z"), time.UTC)
    if got.ActiveWindowMinutes != 5 { t.Fatalf("got %d", got.ActiveWindowMinutes) }
}
```

同时覆盖 30 分钟会话边界、25 分钟专注时段、同会话项目切换、分类优先级和证据最多 50 条。

- [ ] **Step 2: 运行失败测试**

Run: `go -C server test ./internal/effectiveness -v`

Expected: FAIL，缺少 `EventPoint` 和 `AggregateDay`。

- [ ] **Step 3: 实现纯聚合函数**

实现 5 分钟桶去重、按时间排序、会话切分、同项目连续桶、上下文切换和分类计数。函数不依赖数据库或 HTTP。

- [ ] **Step 4: 运行测试**

Run: `go -C server test ./internal/effectiveness -v`

Expected: PASS。

### Task 2：PostgreSQL 日聚合与权限 migration

**Files:**
- Create: `server/migrations/005_personal_effectiveness.sql`
- Create: `server/internal/effectiveness/repository.go`
- Modify: `server/internal/authorization/authorizer.go`

**Interfaces:**
- Produces: `Repository.RecomputeDay(ctx, tenantID, subjectID string, day time.Time, location *time.Location) error`
- Produces: `Repository.ListDaily(ctx, tenantID, subjectID string, from, to time.Time) ([]DailyMetrics, error)`
- 新表：`subject_effectiveness_daily`、`effectiveness_recompute_jobs`、`user_subject_links`。

- [ ] **Step 1: 写 migration 静态测试**

```go
func TestEffectivenessMigrationDeclaresRequiredTables(t *testing.T) {
    sql := readMigration(t, "../../migrations/005_personal_effectiveness.sql")
    for _, name := range []string{"subject_effectiveness_daily", "effectiveness_recompute_jobs", "user_subject_links"} {
        if !strings.Contains(sql, name) { t.Fatalf("missing %s", name) }
    }
}
```

- [ ] **Step 2: 运行失败测试**

Run: `go -C server test ./internal/effectiveness -run Migration -v`

Expected: FAIL，migration 文件不存在。

- [ ] **Step 3: 实现 migration 和 repository**

Migration 插入 `effectiveness:read`、`effectiveness:manage`，只绑定 `platform_admin`、`tenant_admin`、`member`；启用 RLS。Repository 使用 UTC 边界查询事件并 upsert 日聚合。

- [ ] **Step 4: 运行测试和 migration 静态检查**

Run: `go -C server test ./internal/effectiveness ./internal/db -v`

Expected: PASS。

### Task 3：周期报告、趋势与后台重算

**Files:**
- Create: `server/internal/effectiveness/service.go`
- Create: `server/internal/effectiveness/worker.go`
- Test: `server/internal/effectiveness/service_test.go`
- Modify: `server/internal/config/config.go`
- Modify: `server/cmd/aetheris-server/main.go`

**Interfaces:**
- Produces: `Service.Report(ctx, principal, subjectID string, from, to time.Time) (Report, error)`
- Produces: `Service.ListSubjects(ctx, principal, from, to time.Time) ([]SubjectSummary, error)`
- Produces: `Service.RecomputeRange(ctx, tenantID string, from, to time.Time) (string, error)`
- Produces: `Worker.Run(ctx)`，每 5 分钟重算今天和昨天。

- [ ] **Step 1: 写趋势和覆盖率失败测试**

```go
func TestPercentChangeIsNullWhenPreviousIsZero(t *testing.T) {
    if got := PercentChange(10, 0); got != nil { t.Fatalf("got %v", *got) }
}
```

覆盖 `covered_days/period_days`、上一等长周期和 90/31 天限制。

- [ ] **Step 2: 运行失败测试**

Run: `go -C server test ./internal/effectiveness -run 'Trend|Coverage|Range' -v`

Expected: FAIL，周期服务尚未实现。

- [ ] **Step 3: 实现 service、worker 和配置**

新增 `EFFECTIVENESS_TIMEZONE=Asia/Shanghai`、`EFFECTIVENESS_RECOMPUTE_INTERVAL=300`。worker 使用独立 goroutine，错误只写无 payload 日志。

- [ ] **Step 4: 运行 Go 测试**

Run: `go -C server test ./...`

Expected: PASS。

### Task 4：RBAC 与 HTTP API

**Files:**
- Create: `server/internal/httpapi/effectiveness_handlers.go`
- Test: `server/internal/httpapi/effectiveness_handlers_test.go`
- Modify: `server/internal/httpapi/router.go`
- Modify: `contracts/openapi.yaml`

**Interfaces:**
- `GET /api/v1/admin/effectiveness`
- `GET /api/v1/admin/effectiveness/subjects`
- `POST /api/v1/admin/effectiveness/recompute`
- `POST /api/v1/admin/effectiveness/summary`

- [ ] **Step 1: 写参数和权限失败测试**

测试缺少 `subject_id` 返回 400、超过 90 天返回 400、无 `effectiveness:read` 返回 403、设备 principal 返回 403。

- [ ] **Step 2: 运行失败测试**

Run: `go -C server test ./internal/httpapi -run Effectiveness -v`

Expected: FAIL，路由不存在。

- [ ] **Step 3: 实现 handler**

所有查询使用 session principal；`member` 必须匹配 `user_subject_links`；重算只允许 `effectiveness:manage`；模型总结发送聚合投影，不发送原始 payload。

- [ ] **Step 4: 运行测试和 OpenAPI 路径检查**

Run: `go -C server test ./...` 和 `rg '/api/v1/admin/effectiveness' contracts/openapi.yaml`

Expected: PASS 且 OpenAPI 包含四个 endpoint。

### Task 5：Admin Web 个人效能页面

**Files:**
- Create: `admin-web/src/pages/EffectivenessPage.tsx`
- Test: `admin-web/src/pages/EffectivenessPage.test.tsx`
- Modify: `admin-web/src/api/types.ts`
- Modify: `admin-web/src/api/client.ts`
- Modify: `admin-web/src/App.tsx`
- Modify: `admin-web/src/styles.css`

**Interfaces:**
- `api.listEffectivenessSubjects(from, to)`
- `api.getEffectiveness(subjectID, from, to)`
- `api.recomputeEffectiveness(from, to)`
- `api.summarizeEffectiveness(subjectID, from, to)`

- [ ] **Step 1: 写页面失败测试**

测试导航存在“个人效能”，页面显示主体/7/30/90 天筛选、活跃时段免责声明、KPI、趋势、构成、覆盖度和无排名字段。

- [ ] **Step 2: 运行失败测试**

Run: `npm --prefix admin-web test -- --run src/pages/EffectivenessPage.test.tsx`

Expected: FAIL，页面不存在。

- [ ] **Step 3: 实现页面**

使用 CSS 柱状趋势图和比例条，不增加图表依赖；证据 ID 点击后调用现有 `api.getEvent` 显示详情；AI 总结必须由用户点击触发。

- [ ] **Step 4: 运行前端测试和构建**

Run: `npm --prefix admin-web test -- --run` 和 `npm --prefix admin-web run build`

Expected: PASS。

### Task 6：远程 migration、部署与验收

**Files:**
- Modify: `docs/operations/server-deployment.md`
- Modify: `docs/operations/server-implementation-log.md`

**Interfaces:**
- 远程运行 `aetheris-migrate`，部署 Go Server 与 Admin Web，保持 `/opt/aetheris` 回滚包。

- [ ] **Step 1: 本地全量验证**

Run: `go -C server test ./...`、`go -C server vet ./...`、`npm --prefix admin-web test -- --run`、`npm --prefix admin-web run build`。

- [ ] **Step 2: 远程备份并执行 migration**

备份当前 Go binary、Admin dist 和数据库 schema；执行 migration 后确认 `schema_migrations` 包含版本 5 和新表。

- [ ] **Step 3: 并行/重启验证**

部署新 binary/dist，重启 `aetheris-server`，验证 health、Admin 登录、个人效能 API 的空数据/参数错误/权限行为。

- [ ] **Step 4: 浏览器验收**

访问 `/` 登录后确认“个人效能”导航、主体筛选、KPI、趋势、数据不足状态在桌面和移动视口不重叠；控制台无错误。

- [ ] **Step 5: 更新中文记录**

记录 migration 版本、服务状态、API/页面验证和回滚目录；不记录密码、token、事件 payload。

