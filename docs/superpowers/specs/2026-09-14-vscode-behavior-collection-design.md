# Aetheris VS Code 行为采集设计

**日期：** 2026-09-14  
**状态：** 待复核  
**范围：** Aetheris Core、VS Code 扩展、Windows 安装程序、本地 Lens、Go Server、Admin Web、Forge、Work Episode

## 1. 目标

为 Windows 版 VS Code 提供可解释、可关闭、可审计的研发行为采集能力，准确记录用户在已授权项目中的文件打开、编辑、保存、关闭、工作区切换和扩展状态变化。

现有基于窗口标题的原生适配器继续保留，用于判断 VS Code 是否运行以及获取最低限度的窗口状态；真实编辑行为由 Aetheris VS Code 扩展提供。扩展不采集源代码正文、完整差异、剪贴板、按键内容或第三方扩展私有数据。

## 2. 首期边界

首期支持：

- Windows 10/11；
- VS Code Stable；
- 本机 Windows 工作区和多根工作区；
- Git、SVN 和用户明确授权的非版本库项目；
- 默认安装扩展，但允许用户在安装时取消；
- 安装后允许用户从本地 Lens 或 VS Code 中停用、启用和卸载；
- 将启用、停用、未安装、失联和不兼容等状态上传到服务端。

首期不支持：

- VS Code Web；
- 浏览器版 Codespaces；
- WSL、Remote SSH、Dev Container 等远程扩展宿主中的文件行为；
- 监控任意第三方扩展内部动作；
- 采集源代码正文、完整 diff、输入字符内容或终端输出；
- 绕过用户主动取消或停用扩展的选择。

远程扩展宿主被识别时，状态为 `unsupported_remote_host`，不降级读取远程文件。

## 3. 总体架构

```text
VS Code Extension
  -> CurrentUser Windows Named Pipe
  -> Aetheris Core 协议校验、项目授权、脱敏
  -> 有界 SQLite 离线队列
  -> Gateway
  -> Go Server 原始事件
  -> Forge 规范事实
  -> Work Episode / Pulse / Nexus

Windows 原生窗口适配器
  -> VS Code 运行与窗口状态兜底
  -> 不冒充真实编辑行为
```

新增独立模块 `vscode-extension/`，生成单个 VSIX。扩展和 Core 使用版本化协议通信，扩展不直接连接服务器，也不保存设备 Token。

## 4. 安装、取消和卸载

### 4.1 默认安装

NSIS 安装程序显示“安装 VS Code 行为采集扩展”选项，默认选中。选中时，安装程序直接启动 `Code.exe --install-extension <vsix> --force`，不通过 CMD 或 PowerShell，子进程隐藏运行。

VSIX 内嵌在 Aetheris 安装包中，不依赖在线市场。如果安装时未发现 VS Code，则写入 `pending_install`，Core 后续检测到 VS Code 后在本地 Lens 提示用户完成安装，不静默改变已经完成的用户选择。

VSIX 必须具有可验证的发布者和完整性签名。签名缺失、无效或发布者不匹配时终止扩展安装并上报固定错误码，不得关闭 VS Code 的扩展签名验证。

### 4.2 用户取消

用户在安装时取消扩展：

- Core 和服务器记录 `install_declined`；
- 不将该状态显示为错误；
- 本地 Lens 提供“安装 VS Code 扩展”操作；
- 不采集取消期间的历史编辑行为。

### 4.3 停用和卸载

用户可从本地 Lens 停用扩展。Core 将状态写为 `paused_by_user`，扩展停止产生新事件并清空未提交的内存聚合，不追溯停用期间数据。

用户从 VS Code 卸载扩展后，Core 检测到扩展不存在并上报 `not_installed`。Aetheris 不自动重新安装。重新启用或安装必须由用户从本地 Lens 发起。

卸载 Aetheris 时，如果扩展由 Aetheris 安装程序安装，则尝试通过 `Code.exe --uninstall-extension` 移除扩展；失败不阻止 Aetheris 主程序卸载，但写入不含敏感内容的本地卸载结果。

## 5. 行为事件

所有事件使用 AetherisEvent 包装，包含设备、主体、项目、会话、来源版本、发生时间、脱敏报告和证据来源。扩展生成客户端事件 ID，Core 和服务端按 ID 幂等。

### 5.1 文件打开

事件类型：`ide.file_opened`

由 `window.onDidChangeActiveTextEditor` 产生，只记录用户实际切换到的活动编辑器，不使用 `workspace.onDidOpenTextDocument` 直接代表用户打开，避免语言服务后台加载造成噪声。

字段：

- `tool=vscode`；
- 工作区标识；
- 项目相对路径；
- 文件名；
- 扩展名；
- VS Code `languageId`；
- URI scheme；
- 是否只读；
- 发生时间。

仅处理 `file` scheme 且文件位于授权项目根目录内。Untitled、Output、Git 虚拟文档、设置页、扩展页和非文件 URI 不产生文件行为事件。

### 5.2 文件编辑

事件类型：`ide.file_edited`

扩展监听 `workspace.onDidChangeTextDocument`，在内存中按文档聚合，达到以下任一条件时提交：

- 连续编辑空闲 30 秒；
- 文件保存；
- 活动编辑器切换；
- 文件关闭；
- 聚合达到 200 次变更。

字段：

- 项目相对路径、文件名、扩展名、languageId；
- 编辑开始和结束时间；
- 变更次数；
- 新增字符数量；
- 删除字符数量；
- 撤销/重做标记数量（API 可识别时）；
- 聚合触发原因。

`contentChanges.text` 只允许在当前回调内读取字符串长度，禁止写入日志、缓存、事件、异常或调试输出。删除数量仅使用 `rangeLength`。扩展不得保存实际插入或删除文本。

### 5.3 文件保存

事件类型：`ide.file_saved`

字段：项目相对路径、文件类型、保存时间、本次编辑持续时间、累计变更次数、累计新增/删除字符数量。保存事件先刷新对应编辑聚合，再提交保存事件。

### 5.4 文件关闭

事件类型：`ide.file_closed`

字段：项目相对路径、关闭时间、活动持续时间、是否存在尚未提交的编辑聚合。关闭前必须先刷新聚合。关闭事件不记录文件内容。

### 5.5 工作区变化

事件类型：`ide.workspace_changed`

监听工作区文件夹增删和窗口启动，字段包括新增/移除项目标识、工作区类型、发生时间。绝对路径只在扩展到 Core 的本地消息中用于项目授权匹配；Core 上传前移除绝对路径。

### 5.6 扩展变化

事件类型：`ide.extension_changed`

监听 VS Code 扩展集合变化，对前后快照做差异，记录：

- 扩展 ID；
- 版本；
- 安装、移除或版本变化；
- `isActive` 状态变化；
- 发生时间。

`isActive` 只表示扩展已激活，不解释为用户实际使用了某项功能。标准 VS Code API 无法统一监听所有第三方扩展内部命令，因此服务端不得把“已激活”表述为“用户使用了该插件”。扩展清单不得包含扩展配置、认证信息或私有存储内容。

## 6. 项目授权与路径处理

扩展只发送当前工作区和文件绝对路径到本机 Core。Core执行以下处理：

1. 路径规范化并解析符号链接；
2. 确认文件位于已授权项目根目录；
3. 解析设备本地项目 ID 和逻辑项目 ID；
4. 转换为项目相对路径；
5. 执行路径段脱敏；
6. 删除绝对路径；
7. 生成最终 AetherisEvent。

未授权路径、路径逃逸、无法规范化的路径和远程 URI 全部拒绝，拒绝原因进入安全计数，不记录原始路径。

多工作树使用现有 Git 远程指纹归入同一逻辑项目，同时保留项目位置。只显示相对路径，不向管理端显示本机盘符和用户目录。

## 7. 本地通信与离线队列

### 7.1 命名管道

管道名：`\\.\pipe\Aetheris.VSCode.Bridge.v1`

管道由当前用户会话中的 Core 创建，ACL 仅允许当前用户 SID 和 SYSTEM。协议使用长度前缀 JSON 帧，单帧最大 64KB，单批最多 100 条。

握手包含：

- 协议版本；
- 扩展 ID和版本；
- VS Code 版本；
- 本地/远程扩展宿主类型；
- 随机会话 ID。

Core 校验消息类型、字段长度、时间范围、批次大小和路径授权。无效帧断开连接并增加安全错误计数，不回显原始字段。

### 7.2 扩展侧缓存

Core 不在线时，扩展把已经聚合且不含正文的元数据写入扩展 `globalStorageUri` 下的有界队列：

- 最大 8MB；
- 最长保留 24 小时；
- 先进先出淘汰；
- 每条记录包含事件 ID和校验版本；
- Core 确认写入 SQLite 后才从扩展队列删除。

扩展队列禁止保存绝对路径之外的正文型内容；绝对路径在 Core 确认接收后立即删除。扩展停用时不生成新事件，已有已授权元数据可由用户选择提交或清除，默认清除。

## 8. 组件状态

服务端以 `vscode_extension` 作为适配器 ID，独立于 `vscode_window` 原生窗口兜底状态。现有健康字段 `state` 保持兼容，新增 `component_state`、`component_version`、`protocol_version` 和 `last_component_heartbeat_at`，不得把安装选择状态塞入错误码。

`component_state` 枚举：

- `active`：扩展已激活，心跳正常；
- `install_declined`：安装时用户取消；
- `paused_by_user`：用户从 Lens 主动停用；
- `not_installed`：未安装或已卸载；
- `pending_install`：用户选择安装，但尚未发现 VS Code；
- `awaiting_activation`：扩展已安装，VS Code 尚未激活扩展；
- `inactive_in_vscode`：VS Code 正在运行但扩展无心跳，可能被禁用或扩展宿主未加载；
- `bridge_offline`：扩展已连接过，但与 Core 通信中断；
- `unsupported_remote_host`：扩展运行在首期不支持的远程宿主；
- `incompatible`：协议或 VS Code 版本不兼容；
- `error`：协议、队列或解析错误。

标准 VS Code API 无法在所有情况下区分“被禁用”和“扩展宿主故障”，因此统一使用 `inactive_in_vscode`，避免服务端给出不可靠结论。

健康状态映射：

- `active` 映射为健康 `active`；
- `install_declined`、`paused_by_user` 映射为健康 `disabled`；
- `not_installed` 映射为健康 `source_missing`；
- `pending_install`、`awaiting_activation` 映射为健康 `idle`；
- `inactive_in_vscode`、`bridge_offline`、`unsupported_remote_host`、`incompatible` 和 `error` 映射为健康 `error`，并使用固定原因码。

状态上报字段：

- 扩展版本、协议版本、VS Code 版本；
- 当前状态和安全原因码；
- 最近心跳、最近成功事件、最近失败时间；
- 本地缓存数量；
- 已接收、已拒绝、已丢弃计数；
- 不包含路径、文件名和代码内容。

Admin Web 将用户选择状态显示为“用户未启用”，不标红；`inactive_in_vscode`、`bridge_offline`、`incompatible` 和 `error` 显示为需要处理。

## 9. 本地 Lens

本地 Lens 增加“VS Code 采集”区域：

- 显示安装状态、扩展版本、VS Code 版本和最近事件；
- 显示隐私边界；
- 提供安装、启用、停用、卸载操作；
- 提供清除扩展侧未提交缓存操作；
- 显示最近 20 条本机行为元数据，不显示代码正文；
- 所有变更继续使用本地控制会话和 CSRF 防护。

## 10. Forge、Work Episode 与展示

Forge 将上述事件归一化为以下动作：

- 打开代码文件；
- 编辑代码文件；
- 保存代码文件；
- 切换工作区；
- 开发扩展环境变化。

Work Episode 使用文件行为补充“动作”和“验证”，但不把字符数量直接解释为工作量或效能。Pulse 可以展示文件活动覆盖、编辑/保存节奏和项目切换，不展示代码行数排名、字符数排名或员工比较。

Nexus 只索引安全摘要，例如“在 jtagent 项目编辑并保存 Go 文件”，不索引文件路径明细、字符数量或扩展完整清单。

## 11. 安全要求

- 扩展不得拥有服务器 Token；
- 扩展不得直接访问 Gateway；
- 命名管道仅限当前用户和 SYSTEM；
- 不记录源代码正文、完整 diff、剪贴板、按键、终端输出和第三方扩展私有数据；
- 不处理未授权项目路径；
- 绝对路径只存在于设备本地授权匹配阶段；
- 日志只能记录固定错误码和计数；
- VSIX、Core 和安装包必须生成 SHA-256；
- 扩展版本与协议版本不兼容时停止行为上传，不进行猜测解析；
- 用户停用后不得继续采集，也不得事后回补停用期间行为。

## 12. 测试与验收

### 12.1 扩展单元测试

- 活动编辑器切换产生 `ide.file_opened`；
- 后台打开文档不冒充用户打开；
- 编辑变更只统计长度，不保存文本；
- 保存和关闭前刷新编辑聚合；
- 未授权路径不发送；
- 扩展快照只记录 ID、版本和激活状态；
- 重复事件 ID 不重复提交；
- 停用状态不产生新事件。

### 12.2 Core 测试

- 命名管道 ACL、帧大小、批量上限和协议版本；
- 项目路径授权和路径逃逸拒绝；
- 绝对路径在入队前删除；
- Core 离线重连和确认后删除扩展缓存；
- 状态机和服务端健康快照；
- 原生窗口兜底不冒充编辑事件。

### 12.3 安装测试

- 默认选中、取消安装、无 VS Code、多个 VS Code 安装路径；
- VSIX 安装和卸载不经过 CMD/PowerShell；
- 安装包退出后只保留正常 Core/Service/VS Code 进程；
- 升级保留用户启停选择；
- 卸载清理扩展和本地桥接文件。

### 12.4 真实端到端验收

在真实 VS Code 中执行：打开授权项目、打开文件、编辑、保存、切换文件、关闭文件、安装/移除测试扩展、停用/启用 Aetheris 扩展。

验收结果必须证明：

- 服务端事件顺序、项目归属和时间正确；
- 服务端明确显示扩展状态；
- 源代码片段、完整 diff、绝对路径和输入文本未出现在扩展缓存、Core SQLite、日志、网络请求和服务端数据库；
- Work Episode 能引用对应文件行为证据；
- 停用期间没有事件，重新启用后不补采停用期间数据；
- 断网重连后事件幂等且不丢失；
- 客户端总缓存继续满足 512MB/7 天边界，扩展侧满足 8MB/24 小时边界。

## 13. 发布顺序

1. 定义事件和桥接协议；
2. 实现扩展事件聚合及隐私测试；
3. 实现 Core 命名管道接收、授权和状态机；
4. 接入 SQLite/Gateway；
5. 实现服务端状态、Forge 和 Work Episode 映射；
6. 实现本地 Lens 和 Admin Web 状态页面；
7. 将 VSIX 接入 NSIS 安装、升级和卸载；
8. 完成真实 VS Code 端到端验收；
9. 构建签名客户端并部署服务端下载。

## 14. 回滚

- 服务端事件类型向后兼容，旧 Core 不发送新事件；
- 新 Core 在扩展不可用时继续运行其他适配器；
- VSIX 发布失败时回滚安装包，不删除用户已有 VS Code 扩展；
- 协议不兼容时扩展停止上传并保留有界元数据，用户停用时默认清除；
- 数据迁移只新增字段和状态，不覆盖历史事件；
- 发布前保留服务器二进制、Admin Web、安装包和数据库备份。
