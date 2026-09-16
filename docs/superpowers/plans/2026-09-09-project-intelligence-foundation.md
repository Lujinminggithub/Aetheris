# Aetheris 项目智能化基础实施计划

> **供自动化实施者使用：** 必须使用 `superpowers:subagent-driven-development`（推荐）或 `superpowers:executing-plans`，按任务逐项实施。本计划使用复选框跟踪状态。

**目标：** 把现有原始采集流水升级为具备可靠项目归属、可解释工作片段、适配器健康诊断、一次性进程授权和真实格式采集的可运行产品闭环。

**架构：** Windows Core 负责本地项目身份、进程授权、适配器健康和安全事件上传；Go 服务端负责逻辑项目注册、历史归属、管理侧工作片段和覆盖率；本地 Lens 只展示当前用户与设备的个人数据和控制项。原始 `AetherisEvent` 保持不可变，所有修正通过版本化派生记录完成。

**技术栈：** Python 3.12、Windows 原生接口、SQLite、DPAPI、Go、PostgreSQL、React、TypeScript、Vitest、Python `unittest`、Go `testing`、NSIS。

**规格：** `docs/superpowers/specs/2026-09-09-project-intelligence-foundation-design.md`

## 全局约束

- 所有说明、界面文案、错误提示和运维输出均使用中文；协议字段、接口路径、数据库标识和第三方产品名保持固定原值。
- 原始事件不可改写，历史归属和工作片段必须带规则版本及证据引用。
- 同一 Git 远程仓库的克隆目录和工作树默认归入同一逻辑项目，位置维度继续保留。
- 绝对路径、远程仓库地址原文、截图、OCR 帧和被拒绝进程清单不得上传。
- 截图只存在于内存中，任何测试和错误日志都不得落盘截图。
- 未授权进程、未授权项目和非白名单浏览器页面不得产生可上传内容。
- 本地 SQLite 上限保持 512 MB，服务端总磁盘预算保持 100 GB。
- 项目注册、历史回填和派生重算必须幂等、可续跑、可回滚。
- 每个适配器必须以真实脱敏样本完成客户端到服务端验收，不能只依赖模拟测试。
- 本目录没有 Git 元数据，因此每个“提交检查点”执行文件差异检查和完整测试，并在实施日志中记录，不运行伪造的 Git 提交。

---

### 任务 1：本地项目身份契约

**文件：**

- 新建：`src/aetheris/project_identity.py`
- 修改：`src/aetheris/project_registry.py`
- 修改：`src/aetheris/projects.py`
- 新建：`tests/test_project_identity.py`
- 修改：`tests/test_project_registry.py`

**接口：**

- 输入：已授权本地项目根目录、版本化租户项目身份密钥、设备级根目录密钥。
- 输出：`normalize_git_remote(value: str) -> str`、`inspect_project_identity(path: Path, remote_key: bytes, root_key: bytes, key_version: int) -> LocalProjectIdentity`。
- `LocalProjectIdentity` 字段固定为 `local_project_id`、`display_name`、`vcs`、`remote_fingerprint`、`root_fingerprint`、`workspace_kind`、`worktree_name`、`key_version`。

- [x] **步骤 1：编写远程地址规范化失败测试**

```python
def test_https_and_ssh_remote_produce_same_canonical_value(self):
    self.assertEqual(
        normalize_git_remote("https://user:secret@example.com/team/repo.git"),
        normalize_git_remote("git@example.com:team/repo.git"),
    )

def test_remote_canonical_value_contains_no_credentials(self):
    value = normalize_git_remote("https://user:secret@example.com/team/repo.git")
    self.assertEqual(value, "example.com/team/repo")
```

- [x] **步骤 2：运行测试并确认当前失败**

运行：`python -m unittest tests.test_project_identity -v`  
预期：因 `aetheris.project_identity` 不存在而失败。

- [x] **步骤 3：实现最小项目身份模块**

```python
@dataclass(frozen=True)
class LocalProjectIdentity:
    local_project_id: str
    display_name: str
    vcs: str
    remote_fingerprint: str | None
    root_fingerprint: str
    workspace_kind: str
    worktree_name: str
    key_version: int

def normalize_git_remote(value: str) -> str: ...
def inspect_project_identity(path: Path, remote_key: bytes, root_key: bytes, key_version: int) -> LocalProjectIdentity: ...
```

- [x] **步骤 4：扩展项目注册表序列化测试**

验证旧配置能够读取，新配置保存上述安全元数据，配置文件仍只保存在安装目录下，不把密钥写入 JSON。

- [x] **步骤 5：运行本地项目测试**

运行：`python -m unittest tests.test_project_identity tests.test_project_registry tests.test_project_scan -v`  
预期：全部通过。

- [x] **步骤 6：执行检查点**

运行：`python -m unittest discover -s tests -v`，检查无回归后在 `docs/operations/implementation-log.md` 记录结果。

### 任务 2：租户项目身份密钥

**文件：**

- 修改：`server/internal/config/config.go`
- 修改：`server/internal/config/config_test.go`
- 修改：`server/internal/devices/service.go`
- 修改：`server/internal/devices/service_test.go`
- 新建：`server/internal/httpapi/project_identity_key_handlers.go`
- 修改：`server/internal/httpapi/router.go`
- 修改：`src/aetheris/credentials.py`
- 修改：`src/aetheris/tray.py`
- 新建：`tests/test_project_identity_key_sync.py`

**接口：**

- 服务端配置：`AETHERIS_PROJECT_IDENTITY_KEY`，解码后至少 32 字节。
- 设备接口：`GET /api/v1/device/project-identity-key`。
- 响应：`{"key":"base64...","version":1}`。
- 本地存储：`CredentialStore` 使用 DPAPI 保存到独立 `project-identity.credential`，不得进入日志或状态 JSON。

- [x] **步骤 1：编写配置和权限失败测试**

```go
func TestProjectIdentityKeyRequiresAtLeast32Bytes(t *testing.T) { /* 短密钥必须失败 */ }
func TestDeviceProjectIdentityKeyRequiresDevicePrincipal(t *testing.T) { /* 管理员会话不得代替设备凭据 */ }
```

- [x] **步骤 2：运行 Go 测试并确认失败**

运行：`go test ./internal/config ./internal/devices ./internal/httpapi`。

- [x] **步骤 3：实现配置、接口和设备权限**

在 `authorization.Principal` 的设备权限中增加 `projects:register`，接口只返回当前租户的版本化身份密钥。

- [x] **步骤 4：编写客户端同步失败测试**

覆盖首次下载、DPAPI 保存、离线继续使用旧密钥、服务端版本更新和日志不泄漏密钥。

- [x] **步骤 5：实现客户端同步**

Core 心跳成功后检查密钥版本；下载失败只更新安全状态，不停止已授权离线采集。

- [x] **步骤 6：运行双方测试和敏感信息扫描**

运行：`go test ./internal/config ./internal/devices ./internal/httpapi`。  
运行：`python -m unittest tests.test_project_identity_key_sync tests.test_credentials -v`。  
运行：`python -m unittest tests.test_no_sensitive_literals -v`。

### 任务 3：Go 服务端逻辑项目注册表

**文件：**

- 新建：`server/migrations/011_project_registry.sql`
- 新建：`server/internal/projects/model.go`
- 新建：`server/internal/projects/service.go`
- 新建：`server/internal/projects/service_test.go`
- 新建：`server/internal/httpapi/project_handlers.go`
- 新建：`server/internal/httpapi/project_handlers_test.go`
- 修改：`server/internal/httpapi/router.go`
- 修改：`server/cmd/aetheris-server/main.go`
- 修改：`contracts/openapi.yaml`
- 修改：`contracts/admin-api.types.ts`

**接口：**

- 输入：`projects.RegistrationBatch`，包含设备上报的安全项目身份。
- 输出：`projects.RegistrationResult`，返回逻辑项目、项目位置、匹配方式和复核状态。
- 管理接口：项目列表、合并、重新分配，均写审计日志。

- [x] **步骤 1：编写迁移结构测试**

断言 `logical_projects`、`project_locations`、唯一键、租户外键、行级安全策略和索引存在。

- [x] **步骤 2：运行迁移测试并确认失败**

运行：`go test ./internal/db ./internal/projects`。

- [x] **步骤 3：编写项目匹配服务测试**

```go
func TestRegisterGroupsSameRemoteFingerprint(t *testing.T) {}
func TestRegisterDoesNotMergeByDisplayNameAlone(t *testing.T) {}
func TestRegisterIsIdempotentForDeviceAndLocalProject(t *testing.T) {}
func TestRegisterMarksConflictingCandidatesForReview(t *testing.T) {}
```

- [x] **步骤 4：实现迁移、模型和服务**

`Service.Register(ctx, principal, batch)` 必须在一个事务内完成匹配、位置更新、版本检查和审计记录。

- [x] **步骤 5：实现设备和管理员接口**

设备只能注册自身位置；管理员操作必须经过现有权限检查。接口错误返回中文 `message` 和稳定英文 `error` 代码。

- [x] **步骤 6：更新契约并运行测试**

运行：`go test ./internal/projects ./internal/httpapi ./internal/db`。  
运行管理端类型检查：`npm run build --prefix admin-web`。

### 任务 4：Core 项目注册同步

**文件：**

- 新建：`src/aetheris/project_sync.py`
- 修改：`src/aetheris/project_registry.py`
- 修改：`src/aetheris/tray.py`
- 修改：`src/aetheris/status.py`
- 修改：`src/aetheris/local_view.py`
- 新建：`tests/test_project_sync.py`
- 修改：`tests/test_tray.py`
- 修改：`tests/test_local_project_management.py`

**接口：**

- `ProjectSync.build_batch(snapshot, keys) -> dict`。
- `ProjectSync.sync(client, snapshot) -> ProjectSyncResult`。
- Core 状态新增 `project_sync`，包含状态、注册版本、最近同步时间、项目数和安全错误码。

- [x] **步骤 1：编写批次安全性测试**

断言请求包含安全名称和指纹，不包含本地绝对路径、远程地址或密钥。

- [x] **步骤 2：运行测试并确认失败**

运行：`python -m unittest tests.test_project_sync -v`。

- [x] **步骤 3：实现同步器和检查点**

项目配置版本变化、密钥版本变化或服务端要求刷新时触发同步；重复调用必须得到相同批次内容。

- [x] **步骤 4：接入 Core 生命周期和 Lens 状态**

心跳、采集和项目动态增删不得互相阻塞；项目同步失败不得造成安装或 Core 启动失败。

- [x] **步骤 5：运行本地闭环测试**

运行：`python -m unittest tests.test_project_sync tests.test_tray tests.test_local_project_management -v`。

### 任务 5：历史归属试运行、应用和回滚

**文件：**

- 新建：`server/migrations/012_project_attribution.sql`
- 新建：`server/internal/projectattribution/model.go`
- 新建：`server/internal/projectattribution/rules.go`
- 新建：`server/internal/projectattribution/repository.go`
- 新建：`server/internal/projectattribution/service.go`
- 新建：`server/internal/projectattribution/worker.go`
- 新建：`server/internal/projectattribution/rules_test.go`
- 新建：`server/internal/projectattribution/service_test.go`
- 新建：`server/internal/httpapi/project_backfill_handlers.go`
- 新建：`server/internal/httpapi/project_backfill_handlers_test.go`
- 修改：`server/internal/activities/repository.go`
- 修改：`server/internal/aiinteractions/repository.go`
- 修改：`server/internal/retrieval/repository.go`
- 修改：`server/internal/cleaning/repository.go`
- 修改：`server/internal/effectiveness/repository.go`

**接口：**

- `Service.Start(ctx, tenantID, ModeDryRun|ModeApply, ruleVersion) -> Job`。
- `Service.Resume(ctx, jobID) -> Job`。
- `Service.Activate(ctx, tenantID, ruleVersion) error`。
- `Service.Rollback(ctx, tenantID, ruleVersion) error`。
- `Resolver.Resolve(RawEvidence) Attribution`，返回逻辑项目、位置、方法、置信度、复核状态和安全证据。

- [x] **步骤 1：编写归属优先级测试**

覆盖精确绑定、远程指纹、安全标签、授权根、会话关联、时间关联、工具兜底和歧义候选。

- [x] **步骤 2：运行规则测试并确认失败**

运行：`go test ./internal/projectattribution`。

- [x] **步骤 3：实现迁移和纯规则解析器**

纯规则解析器不得访问数据库；仓库负责批量读取和写入，服务负责状态机与启用版本。

- [x] **步骤 4：实现任务续跑与版本切换**

批次默认 10,000 条。失败批次记录最后事件标识；重复应用使用主键去重；只有完成任务才能启用。

- [x] **步骤 5：使查询统一解析逻辑项目名称**

活动、AI 交互、清洗、检索和效能查询统一连接当前启用归属版本。没有映射时返回“未归属 · 工具名”，不得返回裸哈希作为名称。

- [x] **步骤 6：运行 Go 回归测试**

运行：`go test ./internal/projectattribution ./internal/activities ./internal/aiinteractions ./internal/cleaning ./internal/retrieval ./internal/effectiveness ./internal/httpapi`。

- [x] **步骤 7：在服务器执行只读试运行**

部署前只上传新程序和迁移文件到版本目录；先执行 `dry-run`，记录精确归属、推断归属、冲突、未归属数量和预计重算量，不启用规则版本。

- [x] **步骤 8：启用和验证回滚**

备份当前数据库，应用迁移，执行回填，确认任务完成后启用；验证页面不再展示裸项目哈希；切回旧版本验证回滚后再重新启用新版本。

### 任务 6：管理端项目页面

**文件：**

- 新建：`admin-web/src/pages/ProjectsPage.tsx`
- 新建：`admin-web/src/pages/ProjectsPage.test.tsx`
- 修改：`admin-web/src/api/types.ts`
- 修改：`admin-web/src/api/client.ts`
- 修改：`admin-web/src/App.tsx`
- 修改：`admin-web/src/styles.css`

**接口：**

- 使用任务 3 和任务 5 的项目列表、合并、重新分配与回填任务接口。
- 展示逻辑项目、项目位置、来源设备、归属数量、冲突和未归属数量。

- [x] **步骤 1：编写中文界面失败测试**

验证页面显示“逻辑项目”“项目位置”“待复核”“历史归属试运行”，且正常列表不显示裸 `project-*`。

- [x] **步骤 2：运行测试并确认失败**

运行：`npm test --prefix admin-web -- ProjectsPage.test.tsx --run`。

- [x] **步骤 3：实现页面、接口和中文错误状态**

合并和重新分配必须显示影响数量并二次确认；试运行与启用为分离操作。

- [x] **步骤 4：运行管理端测试和构建**

运行：`npm test --prefix admin-web -- --run`。  
运行：`npm run build --prefix admin-web`。

### 任务 7：生产级工作片段

**文件：**

- 新建：`server/migrations/013_work_episodes.sql`
- 新建：`server/internal/episodes/model.go`
- 新建：`server/internal/episodes/builder.go`
- 新建：`server/internal/episodes/repository.go`
- 新建：`server/internal/episodes/service.go`
- 新建：`server/internal/episodes/worker.go`
- 新建：`server/internal/episodes/builder_test.go`
- 新建：`server/internal/episodes/service_test.go`
- 新建：`server/internal/httpapi/episode_handlers.go`
- 新建：`server/internal/httpapi/episode_handlers_test.go`
- 新建：`src/aetheris/episodes.py`
- 修改：`src/aetheris/forge.py`
- 修改：`src/aetheris/local_view.py`
- 新建：`tests/test_episodes.py`
- 新建：`admin-web/src/pages/WorkEpisodesPage.tsx`
- 新建：`admin-web/src/pages/WorkEpisodesPage.test.tsx`

**接口：**

- `Builder.Build([]Fact) []CandidateEpisode` 使用逻辑项目、主体、会话和 30 分钟间隔构建候选项。
- `Service.Recompute(ctx, tenantID, affectedEventIDs) error` 写入新版本并原子切换。
- 每个目标、动作、决策、验证、结果和后续项必须有 `EpisodeEvidence`。
- 本地 `build_local_episodes(events) -> list[dict]` 只处理当前设备近期缓存。

- [x] **步骤 1：编写证据强制测试**

```go
func TestPublishedEpisodeRejectsStatementWithoutEvidence(t *testing.T) {}
func TestCommandInvocationDoesNotImplySuccessfulValidation(t *testing.T) {}
func TestTombstoneCreatesNewEpisodeRevision(t *testing.T) {}
```

- [x] **步骤 2：运行测试并确认失败**

运行：`go test ./internal/episodes` 和 `python -m unittest tests.test_episodes -v`。

- [x] **步骤 3：实现确定性构建器和存储**

先从用户消息形成目标，从规范事实形成动作；只有成功测试、构建、健康检查或明确用户确认才能形成成功验证。

- [x] **步骤 4：实现可选 Ollama 增强**

模型输入只包含已授权脱敏证据；输出使用严格 JSON Schema；无效证据标识对应的陈述全部丢弃；模型失败保留确定性版本并标记待复核。

- [x] **步骤 5：实现本地 Lens 与 Admin Web 页面**

本地 Lens 只能访问当前用户与设备；管理端按主体、项目、日期、状态和复核状态筛选，并可逐项查看证据。

- [x] **步骤 6：运行完整工作片段测试**

运行：`go test ./internal/episodes ./internal/httpapi ./internal/retrieval ./internal/effectiveness`。  
运行：`python -m unittest tests.test_episodes tests.test_forge tests.test_local_view -v`。  
运行：`npm test --prefix admin-web -- WorkEpisodesPage.test.tsx --run`。

### 任务 8：适配器健康诊断

**文件：**

- 新建：`src/aetheris/adapter_health.py`
- 修改：`src/aetheris/status.py`
- 修改：`src/aetheris/tray.py`
- 修改：`src/aetheris/local_view.py`
- 新建：`tests/test_adapter_health.py`
- 新建：`server/migrations/014_adapter_health.sql`
- 新建：`server/internal/adapterhealth/service.go`
- 新建：`server/internal/adapterhealth/service_test.go`
- 新建：`server/internal/httpapi/adapter_health_handlers.go`
- 新建：`admin-web/src/pages/CollectionCoveragePage.tsx`
- 新建：`admin-web/src/pages/CollectionCoveragePage.test.tsx`

**接口：**

- `AdapterHealthRegistry.begin(adapterID, stage)`、`success(...)`、`idle(...)`、`fail(state, stage, code)`。
- 允许状态固定为规格中的八种状态。
- 心跳只发送安全计数和错误码，不发送路径、内容、网址或 OCR 文本。

- [x] **步骤 1：编写状态机和泄漏失败测试**

验证非法状态被拒绝、异常映射到具体阶段、健康 JSON 不含路径和内容、一个适配器失败不影响其他适配器。

- [x] **步骤 2：运行测试并确认失败**

运行：`python -m unittest tests.test_adapter_health -v`。

- [x] **步骤 3：实现本地注册表并逐个接入适配器**

首先接入浏览器、VS Code、Visual Studio、Codex、Claude Code、Cursor、Copilot、Git 和终端；删除统一吞掉异常的路径，改为安全错误码。

- [x] **步骤 4：实现服务端汇总和页面**

区分“健康但空闲”“数据源缺失”“格式变化”和“运行错误”，并与原始事件数量分开展示。

- [x] **步骤 5：运行三端测试**

运行：`python -m unittest tests.test_adapter_health tests.test_browser_window_capture tests.test_vscode_adapter -v`。  
运行：`go test ./internal/adapterhealth ./internal/httpapi`。  
运行：`npm test --prefix admin-web -- CollectionCoveragePage.test.tsx --run`。

### 任务 9：本地进程一次性授权

**文件：**

- 新建：`src/aetheris/processes.py`
- 修改：`src/aetheris/consent.py`
- 修改：`src/aetheris/tray.py`
- 修改：`src/aetheris/local_view.py`
- 新建：`tests/test_process_identity.py`
- 修改：`tests/test_consent.py`
- 新建：`tests/test_process_consent_flow.py`

**接口：**

- `WindowsProcessDiscoverer.discover() -> list[ProcessIdentityObservation]`。
- `ProcessConsentService.observe(identity, user_sid) -> ConsentOutcome`。
- `ProcessConsentService.decide(identity_key, decision, logical_project_ids) -> ConsentGrant`。
- 决定值固定为 `pending`、`allow_global`、`allow_project`、`deny`、`always_ignore`、`default_excluded`。

- [x] **步骤 1：编写身份稳定性失败测试**

验证 PID 改变不重复提醒、可信签名升级不重复提醒、发布者或路径变化重新复核、未签名文件哈希变化重新复核。

- [x] **步骤 2：运行测试并确认失败**

运行：`python -m unittest tests.test_process_identity tests.test_process_consent_flow -v`。

- [x] **步骤 3：实现 Windows 原生元数据读取**

使用无窗口的原生接口读取最小身份信息，不启动 `cmd.exe` 或 PowerShell，不读取未授权进程内容。

- [x] **步骤 4：升级本地授权数据库**

持久保存用户 SID、稳定身份键、首次和最近发现、提醒时间、决定、策略版本及项目范围。旧 `ConsentStore` 数据迁移后继续有效。

- [x] **步骤 5：实现托盘通知和 Lens 待处理列表**

新身份只创建一次通知；通知被关闭后待处理项继续存在；用户可修改和重置决定。

- [x] **步骤 6：接入采集门禁并验证**

授权前、拒绝后和项目有歧义时均不得生成可上传内容。运行：`python -m unittest tests.test_process_identity tests.test_consent tests.test_process_consent_flow tests.test_core_capture tests.test_tray -v`。

### 任务 10：真实格式适配器扩展

**文件：**

- 修改：`src/aetheris/adapters/ai_sessions.py`
- 修改：`src/aetheris/adapters/cursor.py`
- 修改：`src/aetheris/adapters/copilot.py`
- 修改：`src/aetheris/adapters/vscode.py`
- 修改：`src/aetheris/adapters/visual_studio.py`
- 修改：`src/aetheris/adapters/visual_studio_events.py`
- 修改：`src/aetheris/adapters/window.py`
- 修改：`src/aetheris/adapters/browser_uia.py`
- 修改：`src/aetheris/adapters/browser.py`
- 修改：`src/aetheris/adapters/git.py`
- 修改：`src/aetheris/adapters/terminal.py`
- 新建：`tests/fixtures/adapters/README.md`
- 修改：`tests/test_ai_session_adapter.py`
- 修改：`tests/test_copilot_cursor_adapters.py`
- 修改：`tests/test_vscode_adapter.py`
- 修改：`tests/test_visual_studio_adapter.py`
- 修改：`tests/test_visual_studio_events.py`
- 修改：`tests/test_browser_window_capture.py`
- 修改：`tests/test_browser_policy.py`
- 修改：`tests/test_git_adapter.py`
- 修改：`tests/test_terminal_adapter.py`

**接口：**

- 所有适配器实现统一 `collect() -> AdapterBatch`，其中包含记录、检查点和健康快照。
- 每条记录必须包含格式版本、源定位、会话标识、项目线索和脱敏报告。

- [x] **步骤 1：建立人工脱敏真实格式样本清单**

为 Claude Code、Cursor、Copilot、VS Code、Visual Studio 和浏览器分别记录来源版本、样本字段、删除内容和预期事件，样本中不得存在真实凭据、完整路径或个人对话。

- [x] **步骤 2：实现 Claude Code 多版本格式**

先在 `tests/test_ai_session_adapter.py` 增加历史索引、会话 JSONL、嵌套消息和工具结果格式测试；运行该测试确认失败；实现格式检测与项目线索提取；再次运行确认通过。

- [x] **步骤 3：实现 Cursor 完整会话格式**

在 `tests/test_copilot_cursor_adapters.py` 增加 composer、agent、bubble 和工作区关系测试；运行确认失败；实现只读数据库解析和检查点；再次运行确认通过。

- [x] **步骤 4：实现 VS Code 与 GitHub Copilot**

在 `tests/test_vscode_adapter.py` 和 `tests/test_copilot_cursor_adapters.py` 增加真实表结构和桥接事件测试；运行确认失败；实现版本检测、只读解析与健康状态；再次运行确认通过。

- [x] **步骤 5：实现 Visual Studio 事件桥接**

在 `tests/test_visual_studio_adapter.py` 和 `tests/test_visual_studio_events.py` 增加解决方案、构建、调试和测试结果测试；运行确认失败；实现桥接消费、检查点和项目映射；再次运行确认通过。

- [x] **步骤 6：实现并验证浏览器隐私门禁**

非白名单页面在 `allowlist` 阶段停止；白名单页面依次经过 URL、内存截图、最多四帧滚动、OCR、DLP 和事件生成；磁盘扫描确认不存在截图文件。

- [x] **步骤 7：完善 Git 和终端关联数据**

在 `tests/test_git_adapter.py` 和 `tests/test_terminal_adapter.py` 增加分支、验证结果、项目线索和跨源哈希测试；运行确认失败；实现有界只读采集；再次运行确认通过。

- [x] **步骤 8：验证跨源工作片段**

使用一个真实脱敏场景证明用户目标、AI 命令、IDE 修改、测试和 Git 提交被关联到同一逻辑项目和工作片段，并且每个结论可回到源事件。

### 任务 11：发布、部署和完整验收

**文件：**

- 修改：`scripts/build_windows_core.py`
- 修改：`scripts/build_windows_setup.py`
- 修改：`installer/windows/nsis/AetherisSetup.nsi`
- 修改：`deploy/install-server.sh`
- 修改：`deploy/systemd/aetheris-server.service`
- 修改：`deploy/backup-postgres.sh`
- 修改：`docs/operations/implementation-log.md`
- 新建：`docs/operations/project-intelligence-acceptance.md`

**接口：**

- Windows 安装包继续使用 NSIS，安装和运行过程中不显示命令行窗口。
- 服务端发布使用版本目录、健康检查和原子切换，不覆盖 `/opt` 其他内容。

- [x] **步骤 1：执行完整自动化测试**

运行全部 Python、Go 和 Admin Web 测试，运行敏感信息扫描、安装脚本测试和契约检查。

- [ ] **步骤 2：构建并本机安装客户端**

验证安装、升级、卸载、开机启动、服务监管、托盘、Lens、一次性进程提醒、项目动态增删和无命令行窗口。

- [x] **步骤 3：服务端只读预检和备份**

检查 `/opt/aetheris`、磁盘、服务、端口和数据库容量；创建可恢复备份；任何发布文件只进入新的版本目录。

- [x] **步骤 4：部署并执行历史回填**

先部署兼容代码，再运行迁移和试运行；检查冲突后应用并启用新归属版本；随后重算工作片段、检索和 Pulse。

- [ ] **步骤 5：执行产品闭环验收**

从客户端下载产生真实事件，在管理端确认逻辑项目、工作片段、适配器状态和证据；在本地 Lens 确认个人视图、进程授权和项目控制。

- [x] **步骤 6：执行回滚演练和容量检查**

回滚服务端程序与启用规则版本，确认原始事件和本地队列完整；恢复新版本后确认结果一致；验证服务端低于 100 GB、本地数据库低于 512 MB。

## 执行顺序

```text
任务 1-4：项目身份与注册
  -> 任务 5-6：历史回填与项目管理
  -> 任务 7：工作片段
  -> 任务 8：适配器健康诊断
  -> 任务 9：本地进程授权
  -> 任务 10：真实格式适配器
  -> 任务 11：构建、部署、回滚和完整验收
```
