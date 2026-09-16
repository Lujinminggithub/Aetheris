# 浏览器活动采集与展示实施计划

> **执行要求：** 按任务逐项实施并在每个步骤完成后更新复选框；所有功能修改遵循测试先行。

**目标：** 打通租户浏览器白名单配置、Windows Core 动态同步、清洗事实、个人效能和管理后台展示的完整链路。

**架构：** PostgreSQL 保存租户级浏览器采集策略，Go Server 分别向管理员和 Device Token 暴露最小接口。Core 在启动及 heartbeat 周期同步策略，使用现有 UI Automation、OCR 和 DLP 适配器产生 `browser.page_view`；现有清洗事实继续承接事件，效能聚合增加独立浏览器指标。

**技术栈：** Go 1.22、PostgreSQL、Python 3.11、React 19、TypeScript、Vite、Windows UI Automation、tesseract.js。

**设计文档：** `docs/superpowers/specs/2026-09-08-browser-activity-design.md`

## 全局约束

- 所有新增文档和用户可见文案使用中文。
- 浏览器采集默认关闭，只采集管理员明确配置的域名。
- 不上传完整浏览历史、Cookie、凭据或非白名单页面。
- 原始事件不可变；统计优先使用当前清洗事实且排除明确标记数据。
- 不增加排名、评分或绩效判断。

---

### Task 1: 浏览器策略数据库与领域校验

**Files:**
- Create: `server/migrations/010_browser_activity.sql`
- Create: `server/internal/browserpolicy/service.go`
- Test: `server/internal/browserpolicy/service_test.go`

**Interfaces:**
- Produces: `browserpolicy.NormalizeDomains([]string) ([]string, error)`、`Repository.Get`、`Repository.Update`。

- [x] 编写域名规范化、去重、非法输入和上限测试。
- [x] 运行 `go -C server test ./internal/browserpolicy -v`，确认因实现缺失而失败。
- [x] 实现策略模型、校验、PostgreSQL repository 和迁移。
- [x] 再次运行包测试并确认通过。

### Task 2: 管理员与设备策略接口

**Files:**
- Modify: `server/internal/httpapi/router.go`
- Modify: `server/internal/httpapi/device_handlers.go`
- Test: `server/internal/httpapi/browser_policy_handlers_test.go`
- Modify: `contracts/openapi.yaml`

**Interfaces:**
- Produces: `GET/PUT /api/v1/admin/browser-policy`、`GET /api/v1/device/browser-policy`。

- [x] 编写路由、鉴权、校验和响应格式测试。
- [x] 运行目标测试，确认端点不存在时失败。
- [x] 接入 repository、权限、CSRF 和审计日志。
- [x] 运行 `go -C server test ./internal/httpapi -v`。

### Task 3: 个人效能浏览器指标

**Files:**
- Modify: `server/migrations/010_browser_activity.sql`
- Modify: `server/internal/effectiveness/definitions.go`
- Modify: `server/internal/effectiveness/aggregate.go`
- Modify: `server/internal/effectiveness/repository.go`
- Modify: `server/internal/effectiveness/service.go`
- Test: `server/internal/effectiveness/aggregate_test.go`

**Interfaces:**
- Produces: `browser_events` 日指标和 `activity_breakdown.browser`。

- [x] 编写 `browser.page_view` 独立分类与聚合测试。
- [x] 运行目标测试并确认当前落入 other 导致失败。
- [x] 增加浏览器计数、持久化和 API 投影，升级指标定义版本。
- [x] 运行 `go -C server test ./internal/effectiveness -v`。

### Task 4: Windows Core 策略同步

**Files:**
- Modify: `src/aetheris/tray.py`
- Modify: `src/aetheris/server.py`
- Modify: `src/aetheris/status.py`
- Test: `tests/test_tray.py`
- Test: `tests/test_browser_window_capture.py`

**Interfaces:**
- Consumes: `GET /api/v1/device/browser-policy`。
- Produces: 动态 `browser_allowlist`、策略 revision 和非阻塞错误状态。

- [x] 编写策略首次应用、revision 不变、同步失败保留旧配置的测试。
- [x] 运行目标 Python 测试并确认失败。
- [x] 实现 Gateway 请求、原子配置更新和浏览器采集器重建。
- [x] 运行 `python -m unittest tests.test_tray tests.test_browser_window_capture -v`。

### Task 5: Admin Web 配置与浏览器指标展示

**Files:**
- Modify: `admin-web/src/api/types.ts`
- Modify: `admin-web/src/api/client.ts`
- Create: `admin-web/src/pages/BrowserPolicyPage.tsx`
- Modify: `admin-web/src/pages/EffectivenessPage.tsx`
- Modify: `admin-web/src/App.tsx`
- Modify: `admin-web/src/styles.css`

**Interfaces:**
- Consumes: 管理员浏览器策略接口和 `activity_breakdown.browser`。

- [x] 添加 API 类型测试或页面可验证断言，使当前缺失页面时失败。
- [x] 实现导航、开关、域名编辑、保存反馈和独立浏览器指标标签。
- [x] 运行 `npm --prefix admin-web run test`（若项目无测试脚本则运行类型检查）。
- [x] 运行 `npm --prefix admin-web run build`。

### Task 6: 全量验证与生产部署

**Files:**
- Modify: `docs/operations/implementation-log.md`
- Update: `releases/` 中版本化 Server/Admin Web/Setup/Core 产物。

**Interfaces:**
- Produces: 可下载的统一版本客户端和已部署服务端。

- [x] 运行 Python、Go、Admin Web 全量测试与构建。
- [x] 构建版本化 Core 和 Setup，校验哈希及版本一致性。
- [x] 部署数据库迁移、Go Server 和 Admin Web 到 `/opt/aetheris`。
- [x] 验证 `/healthz`、策略接口、活动分类、下载链接和服务日志。
- [x] 将实施结果和验证命令记录到中文运维日志。
