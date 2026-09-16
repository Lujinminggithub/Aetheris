# 服务端实现记录

## 2026-09-06

- 已建立 `server/` Go module，入口为 `aetheris-server` 和 `aetheris-migrate`。
- 已建立 PostgreSQL migration：租户、Admin 访问角色、主体、设备、工作角色、事件、tombstone、模型运行、审计和 session 表，并启用租户 RLS。
- 已实现设备凭证哈希、Admin Argon2id 密码哈希、HttpOnly session 和 CSRF 双提交校验。
- 已实现工作角色优先级：项目覆盖 > 主体分配 > 租户默认；事件保存工作角色快照。
- 已实现 React/Vite Admin Web，可独立开发和构建，Go Server 可托管 `admin-web/dist`。
- 已实现 Python Model Gateway，Ollama 默认，Dify 可选，provider 失败不会阻塞事件 ingest。
- 已添加 systemd、安装、PostgreSQL 备份、Docker Compose 和中文运维文档。

## 验证结果

- `go -C server test ./...`：通过。
- `go -C server build ./...`：通过。
- `go -C server vet ./...`：通过。
- `npm --prefix admin-web test -- --run`：1 个测试通过。
- `npm --prefix admin-web run build`：构建成功。
- `PYTHONPATH=model-gateway python -m unittest discover -s model-gateway/tests -v`：10 个测试通过。
- `python -m unittest discover -s tests -q`：现有 57 个测试通过。

## 环境限制

本地工作机未安装 Docker、`psql`，因此无法在本地执行真实 PostgreSQL migration。已使用临时 SSH 密码完成目标虚拟机的只读验证，未执行写入、安装、替换或重启：

- 系统：Ubuntu 22.04，内核 `6.8.0-110-generic`。
- `/opt/aetheris`：存在，权限 `750`，归属 `aetheris:aetheris`。
- 磁盘：根文件系统 196 GB，已用 77 GB，可用 109 GB（42%）。
- PostgreSQL：未安装，`psql` 不存在，`postgresql.service` 不存在。
- 新 Go 服务：`aetheris-server.service` 尚未安装。
- 当前 8080：旧 Python Gateway 进程监听 `0.0.0.0:8080`，数据库为 `/var/lib/aetheris/server.db`。
- 旧 Gateway：`GET http://127.0.0.1:8080/healthz` 返回 HTTP 200，`{"status":"ok"}`。

因此远程环境验证已完成，但新架构尚未部署到该机器。后续部署必须先规划 PostgreSQL 安装和旧 Gateway 停机/迁移窗口，避免直接占用现有 8080 或覆盖当前 `/opt/aetheris` 内容。

## 2026-09-06 远程迁移结果

- 已在目标 Ubuntu 22.04 虚拟机安装 PostgreSQL 14，并启用 `postgresql.service`。
- 已创建独立 PostgreSQL 数据库 `aetheris` 和应用登录角色；Go migration 执行成功，4 个 migration、21 张 public 表存在。
- 已将 Linux amd64 Go Server、Admin Web 静态资源和 Model Gateway 部署到 `/opt/aetheris`。
- 新 Go Server 先在 `127.0.0.1:18080` 并行验证，再切换到 `0.0.0.0:8080`。
- Model Gateway 运行在 `127.0.0.1:18081`，健康检查通过。
- 旧 SQLite Gateway 数据库检查结果：事件 0 条、tombstone 0 条、设备 2 条；没有需要导入的历史事件。原数据库和配置已备份到 `/opt/aetheris/backups/migration-20260906T125020Z`。
- 旧 Python Gateway 已停止；其回滚 unit `/etc/systemd/system/aetheris-legacy-gateway.service` 已安装但未启用。
- 切换后验证：Go Server 8080 health、Admin 登录、Admin summary、Admin 静态页面、Model Gateway health、PostgreSQL service 全部通过。

## Admin Web 丰富化

- 总览增加主体/设备/活跃设备/今日事件 KPI、最近活动和模型服务健康。
- 增加设备与主体页面，支持搜索、状态和最后心跳展示。
- 事件页面增加工作角色筛选、关键词搜索、详情抽屉、脱敏 payload 展示和归档操作。
- 增加 `/api/v1/admin/summary`、`/api/v1/admin/health/providers`、`/api/v1/admin/events/{id}` 查询接口。
- 前端测试 4 项通过，Vite 生产构建成功。

## 2026-09-06 Admin Web 空白页修复

- 根因：Vite 默认生成 `/assets/...` 根路径，而 Go Server 只托管 `/admin/*`，导致 JS/CSS 请求 404。
- 修复：`admin-web/vite.config.ts` 设置 `base: '/admin/'`，并增加构建路径回归测试。
- 验证：远程 `/admin/assets/index-D7xQcyYL.js` 和 `/admin/assets/index-DsOLMj4s.css` 均返回 HTTP 200。

## 2026-09-06 登录入口与客户端下载

- 根路径 `/` 重定向到 `/admin/`，未登录时直接显示 Admin 登录页，登录后进入总览。
- 登录后的右上角新增“下载客户端”入口，固定调用 `/downloads/client`。
- `CLIENT_DOWNLOAD_FILE` 只允许配置一个具体安装包，服务端不开放目录浏览。

## 2026-09-06 Admin 登录 401 修复

- 根因：Linux root 密码与 Admin 数据库密码是两套独立凭据；Admin 认证服务本身正常，bootstrap 管理员凭据验证返回 200。
- 新增 `aetheris-admin reset-password` 命令，要求至少 12 位密码，使用环境变量接收新密码，重置后撤销旧会话并写审计日志。
- 登录失败响应增加中文 `message`，页面显示“用户名或密码错误”，不再只显示裸 HTTP 401。
- 远程管理员密码已重置；新密码验证返回 200，错误密码验证返回 401。

## 2026-09-06 个人效能模块

- 新增确定性个人效能聚合：5 分钟活跃桶、30 分钟会话边界、25 分钟专注时段、项目上下文切换和活动分类。
- 新增 migration 5：`subject_effectiveness_daily`、`effectiveness_recompute_jobs`、`user_subject_links`，以及 `effectiveness:read`/`effectiveness:manage` 权限。
- 新增个人效能报告、主体摘要、重算和可选 AI 总结 API。
- Admin Web 新增“个人效能”主导航、7/30/90 天周期、六项 KPI、每日趋势、活动构成、项目/工作角色分布、口径、覆盖率警告和证据下钻。
- 不提供员工排名、排行榜、绩效灯或黑盒综合总分。
- 远程 migration 版本为 5，三张新表验证存在；API 验证结果为主体摘要 200、空报告 200、超过 90 天 400、重算 202。
- 部署前备份位于 `/opt/aetheris/backups/effectiveness-20260906T143256Z`。

## 2026-09-07 Windows Setup 与 Core 0.4.0

- 已把安装和运行拆分为 `AetherisSetup-0.4.0.exe` 与 `AetherisCore-0.4.0.exe`。
- Setup 支持用户自选安装目录、递归扫描 8 层内最多 100 个 Git/SVN 项目、多项目确认、主体标识和 enrollment bootstrap。
- device token 使用 Windows DPAPI CurrentUser 加密，配置文件不保存明文 token。
- 安装成功需要 bootstrap、credential round-trip、heartbeat 身份一致和 Core 状态验证全部通过。
- Core 使用用户级 Windows named mutex，注册、采集和队列状态相互独立；heartbeat 使用 5 秒到 5 分钟退避。
- 托盘首次启动发送通知并打开本地状态页；本地状态页提供立即 heartbeat、立即采集、注册/采集/队列状态。
- Go Server bootstrap 支持 `subject_name`，heartbeat 返回 tenant/subject/device/server_time；修复 credential join SQL 列名歧义导致的新 token 误报 401。
- 服务端 ingest 首次接收项目时只登记不透明 `project_id`，不保存客户端项目路径。
- 真实链路验证：bootstrap 200、heartbeat online、ingest accepted，PostgreSQL 中设备/主体/事件/项目均可见，验证数据随后清理。
- Windows 实机验证：Core 注册状态为 `registered`，本地状态页 HTTP 200，通知区登记成功，测试进程和数据已清理。
- 服务端默认 `/downloads/client` 已切换到 `AetherisSetup-0.4.0.exe`，Core/Setup SHA-256 在 Linux 上验证通过；0.3.x 文件保留用于回滚。
- 发布前服务端备份位于 `/opt/aetheris/backups/core-040-server-20260906T154818Z`。
- 构建输出使用独立 `releases/0.4.0/`，避免并行测试清理共享 `dist/` 造成竞态。
- 最终 Setup 会在安装前检测正在运行的 0.3.x/0.4.x Core，要求用户主动退出，不静默终止进程。
- 发布后验证：错误 enrollment 被拒绝；正确 bootstrap、heartbeat 身份、数据库设备可见均通过；默认下载文件名为 `AetherisSetup-0.4.0.exe`。

## 2026-09-07 Setup 0.4.1 项目扫描卡死修复

- 根因：NSIS 在 `StartProjectScan` 插件调用返回时卸载 provisioning DLL；全局 `ProjectScanner` 析构函数在安装器 UI 线程执行 `join()`，把原本的后台扫描重新变成同步等待，250ms timer 无法运行。
- 修复：`StartProjectScan`、`PollProjectScan`、`PopulateProjectList`、`WriteConfiguration` 及后续 provisioning 调用显式使用 `/NOUNLOAD`，确保扫描器和已选项目状态在安装器生命周期内持续存在。
- 项目授权页新增 marquee 进度条，状态每 250ms 显示“已扫描目录”和“已发现项目”；完成、警告、启动失败或离开页面时停止进度条并恢复扫描按钮。
- 原生 `ProjectScanner` 每扫描 16 个目录或项目数量变化时发布增量快照；快照锁只保护短时间复制，不执行文件系统操作。
- 实机使用 `E:\code` 验证：所有 UI 响应采样均为 `Responding=True`，进度条可见，计数更新到已扫描 3712 个目录，最终发现 16 个项目。
- 截图中的未响应旧安装器进程已结束，未进入注册或安装 section。
- 修复版 SHA-256：`1c0f91bd0bea82e6bd54c0ac01666b7cb48f96b0e10720b42f4a47f81d230b54`。
- 服务端默认下载已更新为修复后的 `AetherisSetup-0.4.1.exe`，旧 0.4.0 和早期 0.4.1 文件仍可用于回滚。

### 最终补丁发布 0.4.2

- 0.4.1 坏包和修复包不再共用版本号；扫描修复正式发布为 0.4.2，避免浏览器下载目录和运维记录混淆。
- 0.4.2 实机再次使用 `E:\code` 验证：安装器全程 `Responding=True`，进度条可见，目录/项目计数更新，最终发现 16 个项目。
- `AetherisCore-0.4.2.exe` SHA-256：`7e0b7a0a6cd985990408a46cf72067334ba6741c83c3216edcc0100d21c652ad`。
- `AetherisSetup-0.4.2.exe` SHA-256：`3723c438e1dd9ee567125042c95c0840a1dbc91089496df5a4539c053391d7ad`。
- 服务端默认 `/downloads/client` 已切换到 `AetherisSetup-0.4.2.exe`；0.4.0/0.4.1 继续保留用于回滚。

## 2026-09-07 Windows Setup/Core 0.4.0 与设备链路

- 发布独立 `AetherisSetup-0.4.0.exe` 和 `AetherisCore-0.4.0.exe`；默认 `/downloads/client` 返回 Setup，Core 作为只读 payload 内嵌并安装到用户选择目录。
- Setup 自动写入服务端地址、bootstrap 返回的 tenant/subject/device 身份、DPAPI device credential、多 Git/SVN 项目和本机检测到的 Codex/Claude/Cursor/Copilot 历史源。
- Core 实现单实例、托盘、本地状态页、独立 heartbeat/capture 状态、多项目采集和 AI 历史有界回溯；窗口进程检查未发现 cmd 或 PowerShell 子进程。
- Go Server bootstrap 接收 `subject_name`，heartbeat 返回 tenant/subject/device/work_role/server_time；修复 device credential 联表查询的歧义列名。
- 真实验证：错误 enrollment 返回 401 且不创建设备；正确 bootstrap 返回一次性 token；heartbeat 返回 online；事件首次 ingest accepted、重放 duplicate；测试主体、设备、事件和凭据随后清理。
- 最终服务状态 active，`/opt/aetheris` 占用 961 MB；0.3.x 文件和旧 Go 二进制保留用于回滚。

## 2026-09-07 NSIS Setup/Core 0.4.1

- 新增设备凭据撤销接口 `POST /api/v1/device/revoke`，事务内撤销活动 token 并把设备标记为 revoked。
- 默认客户端下载切换为 NSIS `AetherisSetup-0.4.1.exe`，保留 0.4.0 文件和服务端二进制用于回滚。
- 生产验收覆盖错误 enrollment、bootstrap、heartbeat、ingest、duplicate、revoke 和撤销后拒绝 heartbeat。
- Go 服务 active；Setup/Core/Server 哈希与 staging、本机及 HTTP 下载一致；Aetheris 总占用 1.3 GB。

## 2026-09-07 Admin Web 事件与角色紧急修复

- 事件详情抽屉的文字颜色异常源于全局 `aside` 样式误伤；抽屉现在显式使用深色文字、零内边距和独立宽度，移动端也不再继承导航侧栏布局。
- 事件页空白的根因是服务端 `work_role` 返回角色快照对象，前端按字符串直接渲染导致 React 运行时崩溃；API 适配层现在统一取出角色 `code`。
- 工作角色页从手工填写内部主体 ID 改为加载并选择真实主体；列表同时展示主体名称和 ID。
- 角色分配服务端改为事务写入；提交前验证主体和项目归属，不存在时返回明确 404，不再暴露为笼统 500。同一主体/项目只保留一条当前管理员分配。
- 线上真实主体 `subject-0272952bd943d19b` 已成功分配“研发”角色；有效分配返回 200，不存在主体返回 404。
- 线上事件列表返回 200；个人效能 7/30/90 天的主体列表和报告接口均返回 200。
- 前端 10 项回归测试、Vite 生产构建、Go 全量测试和 `go vet ./...` 全部通过。发布回滚备份位于 `/opt/aetheris/backups/admin-hotfix-20260907T1304`。

## 2026-09-07 AI 总结 502 修复

- 根因一：Model Gateway 进程存活，但目标机未安装 Ollama，`127.0.0.1:11434` 拒绝连接。
- 根因二：Go Server 发送逻辑模型名 `default`，原 Model Gateway 未将其映射到真实 Ollama 模型，且 Ollama provider 没有把结构化效能上下文加入模型消息。
- 根因三：完整报告为 5,821 prompt token，超过 Ollama 默认 4,096 上下文；扩容后，无限制生成又超过原 30/120 秒超时。
- 安装 Ollama `0.33.3` 并以独立 `ollama` 用户运行，只监听 `127.0.0.1:11434`；默认模型为 `qwen3:4b-instruct`（2.5 GB），模型数据位于 `/opt/aetheris/models/ollama`。
- Ollama 发布包通过官方 SHA-256 `c13cea8f3389db4145f8a6cb88d1747242a48639d7c13e3bda7c1ebdc6eebb2f` 校验后安装。
- Model Gateway 新增 `OLLAMA_MODEL`映射、结构化上下文消息和 `/readyz`；Go 管理端健康检查改为读取 provider 就绪状态。
- 模型上下文设为 16,384，单次输出限制为 256 token，模型调用超时可配且默认 120 秒。模型投影排除逐日明细和证据 ID，保留可解释汇总指标。
- 真实端到端验证：Admin 登录 200、个人效能 AI 总结 200，Ollama 在 46 秒内返回 423 字符中文结果，provider/model 为 `ollama`/`qwen3:4b-instruct`，保留“AI 总结，不作为绩效评价”提示。
- 发布回滚备份位于 `/opt/aetheris/backups/ai-summary-hotfix-20260907T1320`。

## 2026-09-07 历史角色与 Codex 数据回填

- 回填前已生成 PostgreSQL custom-format 备份：`/opt/aetheris/backups/role-backfill-20260907T1412/aetheris-before-role-backfill.dump`。
- 事务性将 674 条历史空角色事件回填为“研发”，并写入 `events.role_backfill` 审计日志。后续 Codex 事件通过当前主体角色分配自动固化“研发”快照。
- Codex 缺失的根因是原生 Setup 配置没有 `ai_session_roots`；Core 0.4.3 在字段缺失时自动检测并持久化本地 AI 源。
- 修正 AI 项目提示优先级和 RFC3339 原始时间，不再统一归到第一个项目或采集时刻。
- 修正 Go 规范 JSON 的 Unicode 十六进制转义；先前 38 条非 ASCII 拒绝事件已重新入队。
- Core 对 PostgreSQL `jsonb` 不兼容的 NUL/控制字符执行可审计替换，同时将上传固定分成每批 100 条，避免终端历史与 AI 历史合并后超过 ingest 单批上限。
- 本机 Core 已升级到 0.4.3；初次全量回填完成后 Codex 事件为 13,841 条，历史时间范围为 2026-08-05 至 2026-09-07。最终验收采样时本地与 PostgreSQL 均已增长至 14,077 条，队列 0、拒绝 0，之后会随新 Codex 会话持续增量上传。
- 效能数据已分两段重算；首次重算结果为 AI 协作 13,841、终端操作 696、交付活动 20。最终验收采样时服务端共 14,793 条事件，全部角色为“研发”；新增事件由 5 分钟后台 worker 持续增量重算。
- 0.4.3 安装包 SHA-256：`762d6026eaafe84733aa5619ebfb817ca660fe444a7d4c83ff89625c72a5b0ad`；服务端默认下载已切换到 `AetherisSetup-0.4.3.exe`。
- 本机数据修复备份位于 `D:\FbBrowser\Aetheris\backups\codex-fix-20260907T1430`，服务端 Unicode 修复回滚二进制位于 `/opt/aetheris/backups/unicode-hash-20260907T1425`。

## 2026-09-07 AI 交互三维管理页

- Admin Web 新增“AI 交互”主导航，按设备 → 项目 → 消息角色三层筛选脱敏会话。
- 消息角色稳定归一为“用户发送”、“AI 回复”、“系统消息”、“工具消息”和“未识别”。
- 项目名称从会话工作目录提取末级目录名；API 不返回盘符、用户目录或完整本机路径。
- 新增 `GET /api/v1/admin/ai-interactions`，支持 1 到 90 天日期范围、设备/项目/消息角色筛选、服务端分页（单页最多 100 条）和脱敏内容摘要。
- 完整脱敏内容继续通过现有事件详情接口按单条读取，没有新增第二份消息内容存储。
- migration 6 新增 `events_ai_dimensions_idx` 部分索引，只覆盖 `ai.message` 的 tenant/device/project/message-role/time 维度。
- 真实数据验证：15,257 条 AI 交互，设备 `DESKTOP-1T1HRVF`，项目 `jtagent`，用户发送 5,499，AI 回复 9,758；首页和第二页均返回 50 条。
- 实际 API 响应时间为 0.29–0.43 秒，项目标签泄漏检查为 false。发布回滚备份位于 `/opt/aetheris/backups/ai-interactions-20260907T1528`。
- 本机 Claude Code CLI 历史已检测到；该阶段配置仅启用 Codex，随后已在 0.4.4 自动补入 Claude 来源并完成首批消息和工具调用上传。

## 2026-09-07 事件清洗事实层与 0.4.4 发布

- 新增 PostgreSQL migration 7：`clean_event_facts`、`cleaning_jobs`、RLS、设备/项目/时间索引、质量索引和源事件 GIN 索引。原始 `events` 未更新或删除。
- Core 0.4.4 使用 PowerShell 反引号奇偶规则组装逻辑命令，参数残片支持正向和倒序关联；Codex/Claude 结构化工具调用保存类型、安全摘要和设备内 HMAC，不保存 AI 命令原文。
- Codex 当前 `custom_tool_call name=exec` 使用静态字符串提取；无法静态确定的 automation 标记 `unresolved_automation_tool_call`，进入待确认且不参与效能统计。
- Admin Web 新增“数据质量”，显示规范命令、`merge_method`、规则版本、置信度、原因码和源事件；源事件可展开查看脱敏详情。“AI 交互”新增“AI 工具执行”维度。
- 用户样本 `event-5c4ee64b6c09bf92c94467ce3fa191eb` 与 `event-47023bfa1f504f4078647c8354685305` 已合并为一条 D 盘规范命令，`merge_method=inferred_parameter_join`，原始两条事件保留。
- 全量回填范围为 2026-08-05 至 2026-09-07。验收时 AI 工具 API 为 21,153 条；AI payload 原文字段泄漏 0，AI 事实 `command_text` 非空 0；最终清洗事实 38,578 条，已合并 48 条，待确认 16,615 条，排除效能 16,620 条。
- 个人效能定义升级为版本 2：终端/AI 从当前清洗事实读取，Git/IDE 等继续读取不可变原始证据；无清洗覆盖时兼容旧数据，不在同一事件类型内混用新旧口径。34 天历史已分两段重算完成。
- Core SHA-256：`b8b1ca3650a2e8b8f17ad202efb8aee5fe0ac3285db71ac708a12261415ae981`。Setup SHA-256：`ec24cbbf7f8f5008ceb85c92137f64733fee403be714c2d0d298a49c64fca874`。
- 服务端备份：`/opt/aetheris/backups/clean-facts-20260907T164453`；本机备份：`D:\FbBrowser\Aetheris-backups\pre-0.4.4-20260907T1658`。

## 2026-09-07 统一活动与本地 RAG

- Admin Web 将独立“AI 交互”和“事件”合并为“活动记录”，统一筛选 AI 协作、AI 工具、终端、IDE、浏览器、版本控制和其他清洗事实；旧 API 保留兼容但不再出现在主导航。
- Core 0.4.5 增加 AI 工具 fallback 项目。历史回放后，原始 AI 数据已形成 `Codex`、`Claude Code`、`Cursor`、`jtagent` 等项目维度；fallback 事件通过 `supersedes_event_id` 关联旧归因，完整 cwd 泄漏检查为 0。
- Codex 加速回放处理 99,610 条，新增 65,765 条并稳定去重 33,845 条；Claude Code 处理并新增 2,616 条。生产验收时共有 72,553 条 supersession 关联。
- 清洗规则版本 2 将 Git、IDE、浏览器等纳入统一活动事实；个人效能定义版本 3 已重算 34 天，并避免统一事实和旧原始事件重复计数。
- Qdrant 1.19.1 使用官方 x86_64 musl 包，SHA-256 为 `70a40529e2ebe0a2787d574d3a2e28437cfe94f26f24fa419f6ac57b4ae817c9`。GNU 包虽校验成功，但因要求 glibc 2.38 未安装。Qdrant 仅监听 `127.0.0.1:6333/6334`。
- 同批 16×约600字生产基准：BGE-M3 热态 76.1 秒、1024 维；EmbeddingGemma 热态 24.3 秒、768 维。最终采用支持多语言和代码语料的 `embeddinggemma`，回答模型仍为 `qwen3:4b-instruct`。
- Model Gateway 新增受内部 token 保护的 `/internal/v1/embed`；Ollama 同时保留两个模型，内部超时为 300 秒。索引与查询使用查询优先门，稳态索引批量为 16。
- RAG 使用用户消息、AI 工具、终端和交付活动作为向量锚点，AI 回复按设备/项目/时间邻域加入生成上下文；相同内容哈希只计算一次 embedding。动态 automation、残片和效能排除事实不进入索引。
- 首次回填完成时 PostgreSQL 为 29,221 条 indexed 文档，pending 0、failed 0；Qdrant `points_count` 同为 29,221，维度 768，tenant/device/project/activity/time 五个 payload 索引均完整。
- 最终真实查询按 `queued → embedding → retrieving → generating → completed` 执行，120.6 秒返回 422 字中文答案和 6 条结构化引用，6 条均含邻域 AI 回复，单条上下文不超过 1200 字；引用事件详情不返回 cwd 或 `project_root`。
- 生产验收快照：原始事件 111,601 条、规则 2 清洗事实 87,747 条、效能版本 3 日记录 34 条；AI tool payload 原命令字段 0，AI 事实完整命令字段 0。
- Core 0.4.5 SHA-256：`3b7c25c41810225242f687be4ccc30dfead603e9ac741d0193982092422ecf87`；Setup 0.4.5 SHA-256：`9b595c0957258137acf8ed9cd536142bf2b8a91420d0b426f08ecf4a5bb1ecff`；最终 Go Server SHA-256：`fc4d18e412b416ae1ec862e2b5d04779d8f54c4c3c4770e3762e810bd299d1cc`。
- 发布前备份位于 `/opt/aetheris/backups/unified-rag-20260907T185230`（152 MB）；本机 Core 备份位于 `D:\FbBrowser\Aetheris-backups\pre-0.4.5-20260907T1948`。
