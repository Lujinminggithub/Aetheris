# Aetheris Execution Log

**最后更新：** 2026-09-18

## 2026-09-18 智能查询自动范围与资源治理

- 智能查询默认使用 `scope_mode=auto`，日期、设备和项目不再是提交前置条件；服务端先基于全局关键词候选选择相关逻辑项目，再在该项目内执行向量与关键词混合检索。
- 手动项目和“全部项目”继续保留为高级范围，租户、权限和 RLS 边界不变。
- Admin Web 将检索状态、活动筛选和项目列表分开加载；普通禁用按钮使用禁止光标，仅真实提交阶段显示等待状态。
- analysis 回答新增最小信息量校验和 DLP 专用主题计划；两次生成仍失败时，允许以 low 置信度返回带引用的未验证过程知识，不再退化为空答案。
- Model Gateway 为 embedding 请求设置独立 6 线程上限，回答模型不受影响；过程知识增量检查周期调整为 60 秒。
- 生产原问题“如何在Windows实现DLP功能”自动路由到 `safe`，最终只保留两条 DLP 过程知识，回答覆盖 OCR、规则检测、阻断审计、性能与隐私，不再混入 EDR/WDK 或复述“后续确认”等过程话术。
- 8 条 embedding 生产采样中，Ollama 峰值为 600%、平均约 310%，不再超过 1000%；活动索引 pending/failed 均为 0。
- 部署后 PostgreSQL、Qdrant、Ollama、Model Gateway 和 Go Server 均为 active/enabled；无未结束查询、无超过 5 分钟 SQL、无新增 error 日志。

## 2026-09-17 过程知识增强智能查询

- 新增过程会话、轮次、知识单元、证据、分块、版本状态和回填任务数据模型，原始事件保持不可变。
- supersession 替代事件在没有更强证据时继承旧事件的逻辑项目，避免 `safe` 重新落入 Codex/Claude Code 兜底项目。
- 会话优先使用结构化 `session_id`，用户问题、AI 探索、AI 最终回答、人工确认和验证结果分开分类。
- 长回答按 600 至 1,000 字语义分块，不再只对前 256 字建立向量。
- 新增独立 `aetheris_process_knowledge_v1` collection、PostgreSQL trigram 关键词检索、RRF、知识/会话去重和验证状态排序。
- 智能查询新增回答计划阶段和动态 token 预算；已验证本地结果优先且必须说明适用条件。
- Admin Web 新增“过程知识”，智能查询要求选择逻辑项目或显式全部项目。
- 新增回填与评测导出 CLI；评测数据仅允许 accepted+verified+active 知识。
- 新增 `deploy/pull-models.sh`，Ollama 启动后自动幂等准备并预热 `qwen3:1.7b` 与 `embeddinggemma`；`qwen3:4b-instruct` 作为可选高质量模型保留。
- 生产数据库已执行迁移 020；过程知识当前版本为 8、模式为 shadow。修复“只取全租户最大清洗规则版本”后，`safe` 候选从 235 增至 3,457 条，形成 149 个知识单元和 233 个已索引分块，其中 52 个包含 EDR 过程知识，pending/failed 均为 0。
- 活动索引与过程知识索引共用前台查询锁，智能查询期间不再启动新的 embedding 批次；模型输入限制为排名前 3 条脱敏过程证据，完整证据仍由 API 返回并可展开查看。
- 纯 CPU 虚拟机实测 `qwen3:4b-instruct` 对约 600-token 结构化提示在 9 分钟内仍无法稳定完成，因此在线默认模型调整为 `qwen3:1.7b`。回答计划与正式回答继续保持两阶段、JSON 约束、引用校验和异步进度，不降低证据隔离边界。
- 回答计划由 Go Server 根据问题类型和召回证据确定性生成，Ollama 只执行一次受 JSON Schema 约束的回答；未包含 verified 证据时，schema 从语法层禁止 high 置信度。
- 固定问题“Windows 操作系统如何实现一个 EDR”已在生产链路返回 analysis 回答，覆盖内核采集、用户态代理、检测关联、响应执行和管理闭环，并引用 `safe` 的 EDR 事件存储、Task/WMI 归因、P0/P1 规则与跨重启基线过程知识。
- 生产验收时 PostgreSQL、Qdrant、Ollama、Model Gateway、Go Server 均为 active/enabled；迁移版本 20，Admin 根路径跳转登录页，无 error 日志和超过 5 分钟的数据库查询。
- 已执行真实版本状态演练：版本 8 切换为 canary 10%，回滚到版本 7，再恢复版本 8 shadow；最终状态为 `active_version=8`、`previous_version=7`、`canary_percent=0`。

## 2026-09-16 Codex 多项目公平回填 0.4.15

- 根因确认：旧 Codex 适配器每轮最多读取 500 条，并始终从最近修改的会话文件开始；活跃会话和超大文件长期耗尽额度，其他项目会话发生饥饿。
- 本机共有 368 个 Codex 会话文件，其中 `E:\project\safe` 有 130 个；修复前 checkpoint 仅覆盖 15 个文件，safe 仅 1 个文件部分读取，Desktop-Asist 会话未进入 checkpoint。
- 采集改为双通道：实时通道保留 100 条给最近 5 个会话；历史通道使用 400 条，按持久化游标轮转所有文件；单文件每轮最多 20 条。
- checkpoint 升级为版本 2，保留既有 offsets 和 session metadata，并新增 `backfill_cursor`；Core 重启后继续公平回填，不清空历史，事件 ID 仍按源文件和 offset 幂等生成。
- 适配器健康快照新增 `total_files`、`covered_files`、`pending_files`。正式运行后 368/368 个会话均进入 checkpoint，safe 130/130、Desktop-Asist 1/1。
- 修复前本地 safe 仅有 6,935 条 Codex 事件；升级后 safe 消息和工具调用继续补齐并上传。当前 Core 0.4.15 已注册、监管服务已连接、队列无积压。
- 历史补传事件的发生时间早于服务端 worker 的今天/昨天窗口，因此新增 `aetheris-admin recompute-cleaning`；运维回填按自然日分段，避免大范围命令关联导致 CPU 长时间满载。
- 2026-08-18 至 2026-09-16 清洗事实已重算。近 30 天 safe 现有 1,823 条清洗事实，其中 81 条用户消息；RAG 中 161 条 safe 文档全部 indexed，包含 3 条 DLP 文档，pending/failed 均为 0。
- 原“DLP 的实现原理是什么”查询实际已把 safe 作为证据 1；问题是 safe 近期会话覆盖不完整，后续引用混入其他项目。公平回填和历史重算已补齐该链路。
- `E:\project\protocol\Desktop-Asist` 的 Codex cwd 当前按其 Git 祖先仓库归入 `protocol`；它与已导入的 `E:\project\Desktop-Asist` 是不同本机路径，不属于本次采集遗漏。
- 0.4.15 Setup 已发布到 `/downloads/client`，SHA-256 为 `ec812266b8a0c38935d6846fcf7d7e5d6a5751aa89e71b1db56c08f52f86a611`，长度 69,406,662 字节。

## 2026-09-16 服务端重启与 systemd 自启动核验

- 服务器本次开机时间为 2026-09-16 09:09:35。PostgreSQL、Go Server、Model Gateway、Ollama 和 Qdrant 均在约 09:09:55 由 systemd 自动启动。
- 五个正式 unit 均为 `enabled`，当前均处于 `active`；PostgreSQL 14 cluster 为 `enabled-runtime` 且正在运行。
- Go Server `/healthz`、Model Gateway `/healthz` 与 `/readyz`、Qdrant `/readyz`、Ollama 模型列表及 PostgreSQL `pg_isready` 均通过；本次 boot 的正式服务无 error 级别日志。
- 发现早期遗留的 `aetheris-gateway.service` 仍为 `enabled`，重启后因无法读取旧 bootstrap 文件持续失败重试；该 Python Gateway 已被 Go Server 替代，若成功启动还会争抢 8080 端口。
- 已备份旧 unit 到 `/opt/aetheris/backups/systemd-autostart-20260916/`，并执行 `systemctl disable --now aetheris-gateway.service`。当前状态为 `disabled/inactive`。
- 回滚专用 `aetheris-legacy-gateway.service` 继续保持 `disabled`，不会随系统启动。正式对外入口仅为 `aetheris-server.service`。

## 2026-09-14 进程身份去重与授权迁移 0.4.9

- 根因确认：旧进程身份键包含名称、路径、发布者、文件哈希和签名状态；同一程序升级、重新签名或身份元数据读取变化后会产生新的授权记录。
- 进程身份现严格采用“规范化名称 + 规范化可执行路径”。Windows 路径统一分隔符、折叠 `.`/`..` 并忽略大小写；发布者、文件哈希和签名状态仅作为可更新的安全元数据。
- Core 启动时在本地 SQLite 事务中迁移历史记录。相同名称和路径只保留一条，保留最早/最近发现时间及最新明确决定，待确认状态不会覆盖已有授权或忽略决定。
- 本机真实数据库副本演练由 194 条合并为 182 条，共消除 12 条重复身份；工作相关 33 条保持不变，迁移后名称/路径重复组为 0。
- 运行中数据库迁移后保留工作相关 33 条、永久忽略 136 条；升级窗口新增的进程作为新的待确认记录正常写入。
- Local Lens 增加“已处理”快速筛选，搜索范围明确覆盖进程名称、路径和发布者；每条已处理记录显示决定状态和决定时间，路径无法读取时明确显示“路径未识别”。
- 实机浏览器验证：“已处理”筛选不混入待确认记录，169 条已处理记录均显示决定时间，当前可见名称/路径重复数为 0。
- 当前工作站 Core 已原位升级到 0.4.9，保留 `D:\FbBrowser\Aetheris\backups\process-identity-20260914T1052` 回滚目录；设备注册、24 个项目和本地队列状态正常。
- 0.4.9 Setup 已部署到服务端 `/downloads/client`，SHA-256 为 `bc44b8488492203465d6b4db8a4c851db9685bd0bf468ce50619863a7e48381f`。

## 2026-09-14 VS Code 原生自动采集

- 根因：旧 Core 仅在配置了 `vscode_state_path` 时创建适配器，且适配器依赖不存在的 `vscode-activity.json`，所以 VS Code 已运行但健康快照和事件均缺失。
- Core 现在默认自动识别 `%APPDATA%\Code\User\globalStorage\state.vscdb`，无需插件、脚本或手工配置；旧配置启动时自动补齐路径。
- 新适配器通过 Windows 原生顶层窗口枚举严格识别 `Code.exe`，采集工作区、活动文件名和扩展名变化，不读取源文件正文，VS Code 不必保持前台。
- 同一窗口状态不会重复上报；活动文件或工作区变化会产生新的 `ide.activity`。同名工作树优先关联非 `.worktrees` 主项目根目录。
- 本机实测识别 `proxy.go - sing-box-extern - Visual Studio Code`，生成 `workspace=sing-box-extern`、`active_file=proxy.go` 并关联 `E:\code\sing-box-extern`。
- 最新 NSIS 安装包已部署 `/downloads/client`，SHA-256 为 `d412ff03ea4aab8701fbd09dce7ec9c40fcf432551df714ef2ca95ff06d9e68d`，HTTP 下载长度 69,059,224 字节；Python 全量 202 项通过。

## 2026-09-14 浏览器前台进程展示白名单

- 采集覆盖页面仅将 `chrome.exe` 和 `msedge.exe` 识别为浏览器前台进程；带域名的成功标记（如 `foreground:msedge.exe:baidu.com`）仍显示为采集中。
- `vmware.exe`、`explorer.exe`、`chatgpt.exe` 等其他 `foreground:*` 标记统一显示“空闲 / 未检测到浏览器前台窗口”，格式列为 `-`，页面不展示进程名。
- 服务端保留原始诊断值，便于内部排障，不改变浏览器采集白名单和隐私边界。
- Admin Web 已部署；生产页面引用 `index-D3cqurtp.js`，全量 13 个测试文件、22 项测试通过。

## 2026-09-11 浏览器采集状态与诊断增强

- 浏览器采集成功后现在写入适配器健康快照：状态为 `active`，记录前台浏览器进程、域名和最近事件时间。
- URL 无法读取与非白名单域名分别处理：前者才标记 `url_read/url_unavailable`，后者显示空闲并记录 `not_allowlisted`，不再把正常忽略当成采集错误。
- 当前设备实况为 `foreground:chatgpt.exe`，因此浏览器适配器显示空闲；真实 Edge 窗口在后台时不会生成浏览器事件，这是前台采集边界。
- 最新 NSIS 安装包已部署 `/downloads/client`，SHA-256 为 `67e2b98b1e28b137df292175e6d2805038967908db84b248c07e630a0dad4b01`，HTTP 下载长度 69,056,496 字节。
- 本轮回归：Python 197 项、Admin Web 21 项、Go 全包测试均通过。

## 2026-09-11 效能项目分布名称修正

- 根因：效能聚合的 `project_breakdown` 原先直接使用本地项目 ID，未经过逻辑项目名称映射，因此页面显示 `project-*`。
- Go Server 聚合现在通过 `current_event_projects` 使用逻辑项目 ID进行切换计算、使用逻辑项目名称作为展示键；Admin Web 同时读取逻辑项目及其位置，将历史日报中的本地项目 ID映射为项目名称。
- 最新服务端二进制已部署并通过 `/healthz`；最新 Admin Web 已部署到 `/admin/`。
- 验证：Go 全包测试通过；Admin Web 13 个测试文件、21 项测试通过；效能聚合回归确认 `jtagent` 显示而不泄漏 `project-*`。

## 2026-09-11 服务端重置后的历史自动重放

- 根因：服务端清空后，Core 仍保留 Codex/Claude 的文件偏移检查点和进程内事件指纹，认为历史已处理，因此不会自动重新上报。
- Go Server 心跳新增 `server_event_count` 和 `data_generation`；Core 持久化上次代际，检测到服务端事件总数下降或首次建立同步状态时，自动清空 AI 历史检查点、Copilot/Cursor 内存指纹并重放本地历史。
- 新增 `AISessionAdapter`、Copilot、Cursor 的 `reset_history()`，重放仍依赖服务端 `event_id` 幂等，不会产生重复事件。
- 新版 Go Server 和 Windows Core 已部署；当前服务端已重新收到 `core.ai.codex` 53,633 条、`core.ai.claude_code` 4,893 条、Cursor 460 条和浏览器事件，历史清洗/归属/Work Episode/检索索引已重新完成。
- 本轮回归：Python 196 项通过；Go 全包测试通过；生产服务 `/healthz` 返回 200。
- 自动回放验证：服务端收到 `core.ai.codex` 53,633 条、`core.ai.claude_code` 4,893 条、Cursor 460 条；最新 Core 状态文件已记录服务端事件代际，Codex/Claude 检查点已重新推进。
- 含自动历史重放能力的 NSIS 安装包已推送 `/downloads/client`，SHA-256 为 `82933d53c265e0d79db882866c3b93b558f2aee0b6fac5989de9dcb3408ac3c2`，HTTP 下载长度 69,055,726 字节。

## 2026-09-11 浏览器空闲状态误报修正

- `foreground:explorer.exe` 表示采集周期内前台是资源管理器，不是浏览器 URL 读取失败；Core 当前逻辑会将非 Chrome/Edge 前台记录为 `idle`。
- Admin Web 兼容旧健康快照：浏览器适配器检测到非浏览器 `foreground:*` 标记时显示“空闲 / 未检测到浏览器前台窗口”，不再显示“采集错误”或 `url_unavailable`。
- 当前设备最新服务端快照已恢复为 browser `idle`、`foreground:chatgpt.exe`，错误阶段和错误码均为空；只有真实 Chrome/Edge 前台窗口才会进入 URL、截图和 OCR 流程。
- Admin Web 已重新构建并部署，生产入口 `/admin/` 返回新静态资源；前端回归测试 2 项通过，生产构建通过。

## 2026-09-11 服务端采集数据重置与重新组织

- 按用户确认执行服务端业务采集数据重置；清理前生成 PostgreSQL custom-format 备份 `/opt/aetheris/backups/pre-reset-20260911T143000Z.dump`，约 180MB，`pg_restore --list` 可读（344 项）。
- 清理范围：events、event blobs/tombstones/embeddings、clean_event_facts、项目归属及回填状态、Work Episode 全部派生表、效能聚合/任务、检索文档/任务、模型任务和适配器健康快照。
- 保留范围：管理员账号、租户、主体、设备和设备凭据、逻辑项目/项目注册、浏览器策略、审计日志、Ollama/Qdrant/下载目录及服务器配置。
- Qdrant 活动集合已删除并按 768 维 Cosine 重新创建；PostgreSQL 与 Qdrant 均完成空库校验。
- Core 服务端恢复后自动重新注册并上传；清洗、项目归属和效能任务已提交并完成。当前重新组织结果：events 129、clean facts 122、project attributions 108、Work Episodes 6、retrieval documents/Qdrant points 58。
- 临时 Admin 会话文件已删除；PostgreSQL、Go Server、Qdrant、Ollama 和 Model Gateway 均 active，`/healthz` 返回 200。

## 2026-09-11 进程授权工作台改版

- Local Lens 进程授权页改为紧凑表格布局，增加状态统计、状态筛选、名称/发布者搜索和移动端响应式样式。
- 每条记录增加复选框，支持全选当前筛选结果；批量操作支持工作相关、仅当前项目、不监控、永久忽略和重置。
- 批量请求按身份逐条提交，失败项单独提示并刷新最终状态，不会把部分成功误报为全部成功；单条操作继续可用。
- 真实浏览器 AX 验证已看到统计卡片、筛选下拉、搜索框、全选框、批量操作栏和行内按钮；全量 Python 测试 194 项通过，Go 全包测试通过。
- 最新 NSIS 安装包已同步服务端 `/downloads/client`，SHA-256 为 `e6f952d1d85fe7705cd47647b1a3c6160d27335b8c189e06245f7969c19c17c1`，HTTP 下载长度 69,054,677 字节。

## 2026-09-11 浏览器采集链路修复

- 根因 1：Edge/Chrome 地址栏 UIA 控件实际位于第 8 层，旧代码只遍历到第 6 层，健康状态长期为 `url_unavailable`。
- 根因 2：自动滚动使用了 `uiautomation` 不支持的 `{PGDN}` 名称，第一帧后抛出 `TypeError`，导致浏览器事件整体丢弃；现改为 `{PAGEDOWN}`。
- 根因 3：Windows OCR Node 子进程输出按系统 GBK 解码，中文输出触发 `UnicodeDecodeError`；现固定使用 UTF-8 并以替换模式读取 stderr/stdout。
- 稳定性修复：前台窗口识别后保留 HWND，UIA 地址栏读取和截图均绑定同一窗口句柄，避免轮询期间前台切换造成 URL 与截图控件错配。
- 浏览器相关回归测试 11 项通过；最新 NSIS 安装包已部署到 `/downloads/client`，SHA-256 为 `b7665e366472105993d3d830c644f8056d6adb01635b03a3fdae937d8d24b01f`，HTTP 下载长度 69,052,159 字节。

## 2026-09-11 本地 Lens 进程授权操作修复

- 根因确认：运行中的旧 Core 内嵌页面只有进程列表，且源码脚本缺少 `show`、项目操作、状态、事件和 Work Episode 函数，导致用户看不到可操作入口或其他按钮无响应。
- 已恢复完整本地 Lens 脚本，并在“进程授权”页为每条进程记录提供“工作相关”“仅当前项目”“不监控”“永久忽略”“重置”操作；授权结果写入本机 `process-consent.db`，无需手动编辑配置文件。
- 页面仍只监听回环地址，所有变更请求继续要求本地控制会话；`allow_project` 自动绑定当前活动项目，未知进程身份仍按一次提醒规则处理。
- 0.4.8 Core 和 NSIS 安装包已重新构建；本机构建 Core SHA-256 为 `e03d69ad2c9fa17683d0c9b8a08f0ffb84b1175ae7173c8187fce1ff2afb492d`，Setup SHA-256 为 `52af6450df05be9790cf3d3bbae16689fa072a1caec3ce7ea2cf0a127db91a84`。
- 实机验证：打开 `http://127.0.0.1:15473/` 的“进程授权”页后，真实 AX 树显示授权按钮和进程记录；进程/本地 Lens 聚焦测试 14 项通过，全量 Python 测试 190 项通过。

## 2026-09-11 浏览器前台诊断与固定 Lens 端口

- 复现确认旧 `15473` 页面不响应的直接原因是内嵌 JavaScript 反斜杠转义导致脚本解析失败；新 Core 页面已通过浏览器点击验证，项目添加请求成功并正确更新 revision。
- Local Lens 现在优先固定绑定 `127.0.0.1:15473`，状态文件写入实际地址；浏览器控制台无新增脚本错误。
- 浏览器适配器已提前到采集周期最前，避免 Git/AI 慢扫描阻塞前台浏览器采集；非浏览器前台显示 `foreground:<进程名>`，URL 读取失败显示 `url_read/url_unavailable`。
- 当前本机真实诊断为 `foreground:chatgpt.exe`，Edge 进程处于后台，因此浏览器状态为 `idle`，没有产生浏览器事件。需将白名单 Chrome/Edge 页面置于 Windows 前台保持一个采集周期。
- Work Episode 本地视图已过滤环境上下文和无目标/无动作噪声，显示设备、项目名称和事件数；服务端当前设备记录按 `device_id` 筛选。
- 最新客户端安装包已重新构建并推送，SHA-256 为 `d836ef1dd2fe30e76b56d4b90afeda9f5f02f41876b4c043f51c3c49f15b5329`。

## 2026-09-11 Lens、浏览器与 Work Episode 修复

- 复现并修复 Local Lens 内嵌 JavaScript 反斜杠转义错误；此前错误导致 `addProject is not defined`，项目按钮完全无反应。
- Local Lens 优先固定监听 `127.0.0.1:15473`，端口被占用时才回退随机端口，并把实际地址写入状态文件。
- 项目添加接口通过真实浏览器点击验证成功，revision 正常递增；动态项目状态仍由 Core 热加载。
- 本地个人 Work Episode 过滤环境上下文和无目标/无动作噪声，补充设备标识、项目名称、事件数和有限证据展示。
- 服务端 Work Episode 增加 `device_id` 迁移、筛选和逻辑项目名称查询；当前设备数据已回填，无法推断设备的旧记录保持隔离。
- 浏览器适配器在非浏览器前台报告 `idle`，URL/UIA 读取失败报告 `url_read/url_unavailable`；服务端健康表已出现浏览器行。没有真实 Chrome/Edge 前台窗口时不会生成浏览器事件。
- 本轮回归：Python 187 项通过；Go 全包通过；Admin Web 20 项测试通过；生产构建通过；服务端已同步最新二进制和 Admin Web。

## 2026-09-09 项目智能化基础整改

- 已确认并写入全中文规格与实施计划，固定“同一 Git 远程仓库的克隆目录和工作树归入同一逻辑项目”的身份规则。
- 任务 1 已完成：新增本地项目身份模块，支持 HTTPS、SSH 和 SCP 风格 Git 远程地址规范化、租户级远程指纹、设备级根目录指纹、普通仓库和工作树识别。
- 项目注册表已向后兼容身份元数据；暂停、恢复和替换项目配置时不会丢失项目身份字段。
- TDD 红灯已验证模块缺失和注册表字段缺失；实现后 10 个项目聚焦测试及全部 153 个 Python 测试通过。
- 任务 2 已完成：服务端新增强制的版本化租户项目身份密钥配置和设备专用读取接口，设备凭据新增项目注册权限。
- Core 使用两个独立 DPAPI 文件保存租户仓库身份密钥和设备根目录密钥；网络失败可使用有效缓存，畸形或短密钥必须拒绝，状态与日志不包含密钥材料。
- 任务 2 门禁：全部 159 个 Python 测试和全部 Go 包测试通过，敏感信息扫描通过。
- 任务 3 已完成核心部分：新增 `logical_projects`、`project_locations`、`project_registry_state` 迁移，设备项目注册接口和管理员项目列表接口；注册匹配优先级、同远程归并、仅同名不归并和冲突复核均有 Go 测试。
- 新增 `project_attributions`、归属规则版本、回填任务和当前事件项目安全视图，支持试运行/应用状态机、批次续跑、启用和回滚；服务端后台 worker 使用默认 10,000 条批次。
- 活动查询已开始通过当前项目安全视图使用逻辑项目名称，正常列表不再依赖裸 `project-*` 名称。
- 任务 7 已完成确定性工作片段基础：按逻辑项目、会话和 30 分钟无活动间隔切分，用户消息形成目标，规范化活动形成动作，明确成功结果形成验证；每项均保留事件证据。工作片段数据库迁移已完成，Ollama 增强和界面仍在后续任务。
- 当时回归门禁：Python 164 项通过；Go 全包通过；OpenAPI YAML 解析通过。后续新增能力均按测试门禁逐步更新，真实格式端到端验收仍未提前标记完成。
- 本轮继续完成：Work Episode 服务端持久化读取、证据详情接口、Admin Web“项目管理”“工作片段”“采集覆盖”页面；本地 Lens 增加待确认进程列表和授权写入入口。
- 新增适配器健康注册表，Core 状态包含各适配器快照；浏览器 URL/截图阶段失败可记录安全错误码，不再统一静默丢失。
- 新增本地进程授权协调器：按用户身份和稳定程序身份一次提醒，支持全局/项目范围允许、拒绝、永久忽略、重置和默认排除。
- 本轮回归门禁：Python 175 项通过；Go 全包通过；Admin Web 13 个测试文件、20 项测试通过；生产构建通过；OpenAPI YAML 解析通过。
- 生产部署已完成：服务端新 Linux 二进制、014 迁移、Admin Web 和最新 Windows NSIS 安装包已部署；`/healthz` 返回 200，`/downloads/client` 返回 0.4.8 新包，SHA-256 为 `f64aad589ffbd8feb050f0472cefd865573b52f2aa2913656ab114eb36773d8d`。
- 服务器正式回填已完成：规则版本 1 处理 237,287 条事件，75,204 条归入 3 个逻辑项目，162,083 条保留未归属/待复核；规则版本 2 启用后回滚到规则版本 1，归属数量保持一致。
- Work Episode 后台重算已启动，服务端当前已有 42 条 active 工作片段；Ollama 摘要接口已加入严格证据 ID 校验。
- 健康快照上传接口已完成，Core 心跳后提交安全状态；本地进程原生身份读取、一次性授权和 Lens 待处理操作已接入。
- 真实格式验收样本已覆盖 Claude Code、Cursor、GitHub Copilot、Visual Studio 和浏览器白名单；仍需在真实 Windows 会话中完成 UIA/OCR、签名发布者和各工具版本的长时间验收。
- 本轮 8 项请求执行结果：现有服务器历史归属正式回填并完成规则版本 2 启用/版本 1 回滚演练；Work Episode 后台每 5 分钟重算最近 24 小时并持续写入新版本；Ollama 摘要严格校验输入证据 ID；本地 Lens 已增加个人工作片段页面；Windows 原生进程身份已加入路径、WinVerifyTrust 签名状态、已签名发布者和未签名哈希读取；健康快照已通过 Core heartbeat 上传到服务端；真实脱敏样本已覆盖 Claude Code、Cursor、GitHub Copilot、Visual Studio、Chrome/Edge 白名单门禁；最新服务端和 Windows NSIS 安装包已生产部署。
- 最新生产安装包 SHA-256：`AetherisSetup-0.4.8.exe` = `1427a947873b271b5b54162d9bae678c7c5bd59f838a3ae9cf1223e33ece38ea`；HTTP 下载长度 69,049,954 字节。
- 最终回归门禁：Python 183 项通过；Go 全包通过；Admin Web 13 个测试文件、20 项测试通过；生产构建通过。计划剩余 14 个步骤仅保留真实 Windows 长时间 UIA/OCR、工具版本差异和完整 Beta 验收，不代表基础链路未实现。
- 服务端 Ollama 重算已部署，配置 `OLLAMA_URL` 时每 5 分钟工作片段重算会尝试证据约束摘要；服务不可用时保持确定性投影。
- 个人 Lens 页面、项目管理、采集覆盖和进程授权 UI 已包含在最新 Core 构建中；最新客户端安装包已重新上传并验证 HTTP 哈希。

## Phase status

- **P0 baseline:** complete. Legacy Windows path migration, admin HTML error handling, direct EXE entrypoint, and baseline regressions are green.
- **P1 reliable delivery:** in progress. Canonical schema, SQLite WAL queue, watermarks, exponential release backoff, idempotent registration/upload, local history, and tombstones are implemented; full multi-device replay tooling remains.
- **P2 process authorization:** in progress. Basic classification, project authorization, and persistent consent grants are implemented; signed publisher verification and Lens pending-process controls remain.
- **P3 capture adapters:** in progress. Git, PowerShell/Windows Terminal history, VS Code activity state, bounded Codex/Claude JSONL history backfill with restart checkpoints, current Cursor numeric composer records, Copilot/Cursor SQLite, Visual Studio solution metadata and debug/build/test bridge events, Windows UI Automation URL reads, in-memory browser screenshots/scroll stitching, and packaged tesseract.js OCR/DLP are implemented; live browser automation and additional tool-version variance remain under acceptance.
- **P4 Forge/WorkEpisode:** Beta grouping projection implemented with evidence IDs; correction-aware recomputation remains.
- **P5 Nexus/logs/Pulse/dataset:** server event exports, daily log, explainable Pulse summary, and dataset manifest are implemented; model-backed retrieval and full rights manifests remain.
- **P6 Lens:** local loopback workspace, server admin login with forced first-password change, device management page, and authenticated event deletion are implemented; full correction/reassignment/grant controls remain.
- **P7 server deployment:** Go Server 0.4.0、独立 Setup/Core 和 Admin Web 已部署到 `192.168.78.138:/opt/aetheris`，服务以 `aetheris` 账户运行。
- **P8 security acceptance:** in progress; automated redaction and secret-literal checks are green, full Beta protocol remains.

## Current verification

- 本地 Python 测试套件：142 项通过；Go 除 Windows 环境拒绝执行自动命名的 `workroles.test.exe` 外全部通过，该包改名编译运行后 2 项通过；Admin Web 17 项测试与生产构建通过。
- Latest admin regression suite covers failed login, forced password change, HTML validation errors, logout, and tombstone deletion.
- Server health: `GET /healthz` returns HTTP 200.
- Server admin shell: `GET /admin` returns HTTP 200 and requires admin login for data.
- Direct client download: `/downloads/client` 返回 `AetherisSetup-0.4.0.exe`，完整 HTTP 流哈希与本机构建一致。
- Latest artifacts: `AetherisSetup-0.4.0.exe` 内嵌 `AetherisCore-0.4.0.exe`；均为 windowed 单文件 EXE，实机进程树没有 cmd/PowerShell 子进程。
- AI 历史：本机只读抽样验证 Codex/Claude 各 100 条、Cursor 100 条；Codex 历史首批 500 条均带 session ID 与项目目录，检查点支持重启续读。
- Live server acceptance: 错误 enrollment 401 且不创建设备；正确 bootstrap、身份化 heartbeat、首次 ingest accepted、重放 duplicate 均通过，测试数据已清理。
- Server disk usage: `/opt/aetheris` 961 MB，低于 100 GB 硬限制。

## 2026-09-07 NSIS 0.4.1

- 默认下载已切换为真正的 NSIS MUI2 `AetherisSetup-0.4.1.exe`；Python/Tk Setup 不再作为生产入口。
- 原生 Win32 Unicode `AetherisProvisioning.dll` 在 NSIS 进程内完成异步 Git/SVN 扫描、WinHTTP bootstrap/heartbeat、DPAPI credential、Core 状态验证和卸载 revoke。
- Setup 是单个可见进程，无 cmd、PowerShell 或 Python Setup 子进程；安装完成后 Setup 退出，只保留逻辑 Core。
- 首次项目选择可跳过；零项目 Core 状态为 `waiting_for_project`。本地 Lens 支持 revision 化添加、暂停、恢复和移除，Core 无需重启即可热加载。
- 本机 fake Gateway 完成 Setup 退出码 0、Core registered、动态项目 0→1→0、原生卸载、Core 正常退出、credential 删除和服务端 revoke 验证。
- 生产服务完成错误 enrollment 401、bootstrap、heartbeat online、ingest accepted、duplicate、revoke 204 和撤销后 heartbeat 401 验证。
- 自动化验证：Python 109 项、C++ provisioning、Go test/vet、Admin Web 7 项和生产构建通过。
- 最终 `/opt/aetheris` 占用 1.3 GB，低于 100 GB 限制。

## 2026-09-08 Windows 服务监管 0.4.8

- 已实现 x64 GUI 子系统 `AetherisCoreService.exe`、活动控制台 WTS 用户启动、受限命名管道 IPC、异常恢复/退避、正常退出抑制、原生 SCM 安装和 NSIS 原子回滚。
- 服务改为 `/MT` 静态链接，发布产物只依赖 Windows 系统 DLL；Python、Go、Admin Web、Core Service 和 provisioning 测试均已通过。
- 本机测试证书的普通 Authenticode `/pa` 验证通过。先前把 `/kp` 失败当作用户态服务阻塞条件属于错误归因；`/kp` 面向内核模式签名。SCM 的 `ERROR_ACCESS_DENIED/FILE_NOT_FOUND` 继续按最终镜像路径、父目录遍历 ACL 和加载环境排查。
- ProcMon 完整捕获确认 `services.exe` 成功映射并创建服务进程，随后 `360Tray.exe` 修改最终文件 DACL/Owner 并成功将服务 EXE 标记删除；SCM 错误来自第三方端点防护，不是 Windows 服务权限或签名策略。原始大体积 PML/CSV 在证据摘要落盘后删除。
- 失败服务已通过 Setup cleanup 模式清理：SCM 注册、64 位 HKLM Core 服务配置和服务 EXE 均已删除，仅保留受保护的空 Service 目录。当前用户态 Core 0.4.7 已恢复并保持 heartbeat registered，设备凭据未改变。
- 早期失败安装覆盖了项目授权列表，当前项目数为 0，且本机没有可恢复的配置备份；项目需从本地 Lens 动态重新添加。
- 发布前验证：Python 146 项、Admin Web 17 项、Go 全包与 vet、Core Service CTest、provisioning CTest 和 NSIS 编译均通过。最终 Setup SHA-256 为 `139c1b18e55024e560430e586e9bd4323eec2b1f4e469ee5092ab8eff8ea6744`。
- 服务器发布回滚目录为 `/opt/aetheris/backups/client-0.4.8-20260908T2210`；staging 与公开下载文件校验一致，`CLIENT_DOWNLOAD_FILE` 已切换为 0.4.8。
- HTTP 验证：`/healthz` 200、`/admin/` 200、`/downloads/client` 200，Content-Disposition 为 `AetherisSetup-0.4.8.exe`，下载长度 68,991,413 字节且 SHA-256 与本机构建一致。发布后 `/opt/aetheris` 占用 6.8 GB。
- 安装尾阶段 `config_update_failed` 的根因是原生更新器只识别无空格的 `"core_version":"..."`，而 Core 持久化的是格式化 JSON。更新器现按顶层 JSON 字段扫描，支持空白/换行/转义，拒绝重复键并保留其他字段。
- 修正版 Setup SHA-256 为 `23bfca19961098d0589a7c118aef7b36582fbb6717a4526caf209943e2137aa5`，HTTP 下载长度 68,991,627 字节且哈希一致。替换前备份位于 `/opt/aetheris/backups/client-0.4.8-config-fix-20260908T2225`。
- 周期性打开本地 Lens 的根因是服务每约 63 秒启动重复 Core，而受监管重复实例仍调用 `webbrowser.open`；服务升级后还会遗留未被新服务接管的 PyInstaller 子进程。Core 现对 `--supervised` 重复启动保持静默，服务按可执行路径、用户 SID 和 SessionId 接管现有 Core，可信 heartbeat 可完成 ready 状态。
- 弹窗修复版 Setup SHA-256 为 `926d3650a0d995ac83cf256f6392ac97262c642cae780d0482463dc8e167e42b`，HTTP 下载长度 68,991,613 字节且哈希一致。替换前备份位于 `/opt/aetheris/backups/client-0.4.8-popup-fix-20260909T0943`。

## 2026-09-08 浏览器活动 0.4.7

- 新增租户级浏览器采集策略和 `/api/v1/admin/browser-policy` 管理接口；策略默认关闭，启用时必须配置最多 100 个规范域名。
- 新增 Device Token 保护的 `/api/v1/device/browser-policy`。Core 在 heartbeat 成功后同步 revision，失败时保留最后一次有效策略并在本地 Lens 状态中记录错误。
- Chrome/Edge 仍只采集前台白名单页面；上传域名、脱敏路径和经 DLP 处理的可见内容，不读取浏览历史、Cookie、凭据或非白名单页面。
- 浏览器页面证据改为每个 5 分钟窗口最多一条，同一页面跨窗口可继续形成活跃证据。
- 个人效能定义版本升级到 4，新增独立 `browser_events` 和“浏览器活动”，不再归入“其他”。迁移复制了 35 条历史日聚合，升级后 30/90 天页面不会变空。
- Admin Web 新增“浏览器采集”入口，提供启用开关、域名列表、策略 revision 和数据边界展示。
- 生产 Go Server、Admin Web 与客户端下载已经切换到 0.4.7；服务端 Setup SHA-256 为 `0f8a70b2842ea15f985609b7d9f75b71550fb64df94fd62ead5ebbff183786b7`。
- 当前工作站 `D:\FbBrowser\Aetheris` 已原位升级到 Core 0.4.7，保留 `AetherisCore.exe.pre-0.4.7` 回滚文件；设备 heartbeat 正常、16 个项目继续采集、队列无积压。
- 生产策略当前为 revision 0、关闭状态。管理员需要在“浏览器采集”页面明确配置域名后，Core 才会采集浏览器活动。

## 2026-09-14 VS Code 行为采集与闭环 0.4.9

- 新增 VS Code 扩展协议、文件行为采集、编辑聚合、扩展清单变化和本地有界 Spool；扩展不读取或保存源代码正文。
- Core 新增 Windows 用户命名管道桥接，按授权项目根目录相对化事件，并支持本地 Lens 安装、启用、停用、卸载和清除缓存控制。
- Adapter Health 新增组件状态、版本、协议、心跳和缓存计数；Admin Web 新增按设备查看 VS Code 组件状态页面。
- Forge、Work Episode、Nexus 和 Pulse 已接入 VS Code 事件：打开/编辑/保存计数可重算，Work Episode 生成安全动作，检索文本不含路径和字符数量。
- 新增数据库迁移 `017_vscode_effectiveness_metrics.sql`，指标定义版本升级到 5。
- 自动化验证：Python 222 项、Go 全包、Admin Web 23 项、VS Code 扩展 16 项全部通过；VSIX、Core、Service、NSIS 安装包已生成。
- 本次构建的 Service 使用本机测试 Authenticode 证书签名并通过 `signtool verify /pa`；正式外发仍需替换为企业证书。

## 2026-09-14 客户端命令来源校准 0.4.10

- 新增客户端 `CommandOriginCorrelator`：只有 AI Shell 事件与终端事件的设备、项目、命令 HMAC 和 30 秒窗口全部匹配时，才抑制重复的 `terminal.command`。
- 关联缓存跨越相邻采集周期，Codex/Claude Code 工具事件先到、终端历史后到时仍可去重；超过 30 秒自动失效。
- 新增 Windows 原生 ToolHelp 父进程链读取，仅上传受限进程名、父 PID 和来源分类，不读取命令行；父进程链只作为辅助证据，不能单独删除终端事件。
- 用户直接运行的 PowerShell/CMD、项目不一致、哈希缺失和时间窗口不匹配均保留。
- Core 本地状态累计显示 `suppressed_duplicate_terminal`，终端 Adapter Health 使用 `discovered/parsed/skipped` 上报每轮发现、保留和抑制数量。
- 发布前只读基线：服务器最近 24 小时 `ai.tool_call=738`、`ai.message=244`、`terminal.command=35`、`process.observed=29`；本次改动只抑制可证明重复的终端事件，不删除 AI 原始证据。
- Python 236 项及 Go 全包测试通过；`AetherisSetup-0.4.10.exe` 已生成，Service 通过 Authenticode `/pa` 验证。

## 2026-09-15 工作进程 OCR 保底与简洁智能查询

- 新增应用 OCR 保底：授权前台工作进程在专属适配器连续 60 秒无有效数据后采集一帧客户区，使用现有 Tesseract.js Worker、DLP 和脱敏链路，截图永不落盘。
- 新增应用 OCR 租户总开关、设备策略同步、健康统计和中文管理端展示；新增迁移 018。
- 新增 `application.activity` 清洗事实、Episode 安全动作和 Nexus 安全摘要，指标口径升级并避免与专属事件重复计算。
- Nexus 智能查询新增问题粒度分类、结构化回答、一次纠错重试、固定中文降级和依据折叠；新增迁移 019。
- 验收文档：`docs/operations/application-ocr-fallback-acceptance.md`。

## 2026-09-15 0.4.12 发布

- Core 已接入应用 OCR 保底：授权前台工作进程 60 秒无专属数据后执行单帧内存 OCR，截图不落盘，DLP 阻断和失败均只保留固定原因码。
- Nexus 已切换结构化回答：问题自动分类，直接问题默认短答案，引用折叠，非法模型输出一次重试后确定性降级。
- 发布前验证：Python 258 项、Go 全包、Admin Web 25 项、VS Code 扩展 16 项全部通过；扩展类型检查、构建和运行时依赖审计通过。
- 生成并发布 `AetherisSetup-0.4.12.exe`；Service Authenticode `/pa` 验证通过。
- 服务端已备份并部署迁移 018/019、Go Server、Admin Web 和客户端下载；服务 `aetheris-server.service` active，`/healthz` 返回 200。
- 下载文件长度 69,403,452 字节，SHA-256 为 `12928585d2326b3c80ed50378cfdc768dd77e5b252cc37c7a0b4614618a2befc`。

## 2026-09-16 已删除项目启动修复 0.4.13

- 根因是 Core 配置加载对旧兼容字段 `project_root` 执行目录存在性硬校验，已删除目录会在托盘初始化前抛错，服务监管随后重复拉起并持续弹窗。
- Core 现仅将存在的项目目录载入运行时；主根缺失时自动选择下一个可用项目，全部缺失时进入等待项目状态。
- 项目注册表保留缺失项目并标记 `available=false`，本地 Lens 显示“目录不存在”并允许直接移除。
- 项目身份同步跳过缺失目录，不再使整个 heartbeat 同步失败。

## 2026-09-16 服务安装与 VS Code 扩展超时修复 0.4.14

- SCM 事件和安装阶段证明服务注册成功；实际失败点是 Core 构造阶段同步调用 VS Code GUI 入口，60 秒超时使 Core 无法在 NSIS 的 45 秒验收窗口内 ready，随后安装器回滚服务。
- VS Code 扩展安装改为直接调用 `Code.exe` 与 `cli.js`，设置 `ELECTRON_RUN_AS_NODE=1`，不调用 CMD 或 PowerShell。
- Core 先向 Windows Service 发送 ready，再异步安装扩展；扩展超时只写固定错误码并按五分钟退避，不再导致 Core 或服务安装失败。
- 本机隔离扩展目录真实验证：CLI 292ms 返回 0，成功安装 `aetheris.aetheris-vscode-0.1.0`。
- Windows 构建脚本现对最终 NSIS 安装器执行 Authenticode 签名和 `/pa` 验证，并在签名完成后生成 SHA-256，避免清单对应未签名文件。
- 发布前验证：Python 267 项、Go 全包、Admin Web 25 项、VS Code 扩展 16 项全部通过；Admin Web 与扩展生产构建、扩展类型检查均通过。
- `AetherisCoreService.exe` 与 `AetherisSetup-0.4.14.exe` 均通过本机测试证书 Authenticode `/pa` 验证；正式外发仍需替换为企业代码签名证书。
- 生产下载入口已切换为 `AetherisSetup-0.4.14.exe`，服务器回滚目录为 `/opt/aetheris/backups/client-0.4.14-20260916-1152`；下载长度 69,408,000 字节，SHA-256 为 `818faacc29a210e10dea664e6f44aedc2f06c43a347b1a3757eb8669ecbe0f69`。

## 2026-09-16 个人效能聚合修复与历史回填

- 原始事件、清洗事实和 Work Episode 均未丢失；页面为空源于个人效能物化聚合连续失败。
- 根因是效能事件 CTE 未声明 `project_id/project_name` 列名，最终投影又遗漏 `project_name`，导致 PostgreSQL 查询和 PGX 扫描契约不一致。
- CTE 列声明、最终 SELECT 和扫描顺序现统一使用受测试保护的 8 字段投影。
- `aetheris-admin recompute-effectiveness` 新增 31 天有界分段和任务完成等待，用于不依赖网页登录凭据的服务器本地历史回填。
- 发布前 Go 全包测试通过；生产服务启动后的自动重算完成，历史回填 5 个分段任务全部完成。
- 版本 6 日聚合共 132 天，覆盖 2026-05-08 至 2026-09-16；最近 30 天为 23 个活跃日、9,885 分钟活跃窗口、50,815 个分类事件。
- 服务端二进制和 PostgreSQL 回滚备份位于 `/opt/aetheris/backups/effectiveness-fix-20260916-1303`；部署后 `/opt/aetheris` 总占用 8.2 GB。
