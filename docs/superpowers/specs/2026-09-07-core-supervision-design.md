# Aetheris Core 自启动与自动恢复设计

**日期：** 2026-09-07  
**状态：** 已被 `2026-09-08-windows-service-supervision-design.md` 替代  
**目标版本：** 0.4.6  
**范围：** Windows 用户登录自启动、异常退出自动重启、正常退出语义、NSIS 安装/升级/卸载集成和生命周期诊断。

> 目标机器同时拒绝原生 Task Scheduler 注册和提权 XML 导入，因此本方案不再作为发布架构。0.4.8 起采用 LocalSystem 监管服务加交互用户 Core 的双进程设计；本文件仅保留问题分析和历史决策。

## 1. 问题与目标

当前正式安装位于 `D:\FbBrowser\Aetheris`。0.4.4 Core 的最后状态为 `registered/running`，Windows Application Error 和 WER 没有崩溃记录，但 Core 进程已经退出。系统中同时缺少 HKCU Run、自启动快捷方式和 Aetheris 计划任务，因此退出后不会恢复。

0.4.6 引入用户级 Task Scheduler 监督。目标是：

- Windows 用户登录后自动启动 Core。
- Core 异常退出后每 1 分钟自动重启。
- 用户从托盘选择“退出”时不自动重启。
- 安装和运行过程不调用 cmd、PowerShell 或 `schtasks.exe`。
- 不使用 Session 0 Windows Service，保证托盘位于当前交互用户会话。

“开机自启动”在托盘产品中的准确语义是用户登录 Windows 后启动。用户尚未登录时不存在可显示托盘的交互会话，Core 不在 Session 0 启动。

## 2. Task Scheduler 定义

任务路径和名称固定为：

```text
\Aetheris\Aetheris Core
```

任务由 `AetherisProvisioning.dll` 使用 Task Scheduler 2.0 COM API 注册，属性如下：

- Principal：当前 Windows 用户。
- LogonType：InteractiveToken。
- RunLevel：LUA，任务启动的 Core 不提升权限。
- Trigger：当前用户登录触发，延迟 10 秒。
- Action：直接运行 `<安装目录>\AetherisCore.exe`。
- Arguments：`--config "<安装目录>\config\aetheris.json" --supervised`。
- WorkingDirectory：安装目录。
- MultipleInstances：IgnoreNew，由 Core named mutex 提供第二层保护。
- StartWhenAvailable：启用。
- DisallowStartIfOnBatteries：禁用。
- StopIfGoingOnBatteries：禁用。
- ExecutionTimeLimit：无限制。
- RestartInterval：1 分钟。
- RestartCount：255（Task Scheduler schema 的 unsignedByte 上限）。

任务定义不包含 enrollment code、device token、control token 或服务器密码。路径和参数由 COM 属性分别设置，不拼接为 shell 命令。由于目标机器策略拒绝中等完整性进程创建计划任务，NSIS 安装/修复时允许出现一次 UAC；提升仅用于任务注册和安装文件维护，不改变任务的 LUA 运行级别。

## 3. 退出语义

Core 退出码定义：

- `0`：用户通过托盘或受保护本地 Lens 明确退出，不触发 restart-on-failure。
- `1`：启动、配置、credential 或未处理运行时错误，允许 Task Scheduler 重启。
- Windows 异常终止码：崩溃或 Task Manager 强制结束，允许 Task Scheduler 重启。

系统关机或用户注销时，当前任务实例结束；下一次用户登录由 LogonTrigger 创建新实例。单实例 mutex 保证登录触发、安装后首次启动和手工启动不会产生多个逻辑 Core。

Task Scheduler 只处理进程退出，不处理仍存活但完全无响应的进程。本期不加入第二个常驻 watchdog 进程；Core 状态时间戳超时监控作为后续可靠性能力。

## 4. 原生插件接口

`AetherisProvisioning.dll` 新增 NSIS 导出：

- `InstallStartupTask`：参数为安装目录，创建或覆盖当前用户任务。
- `RemoveStartupTask`：删除 `\Aetheris\Aetheris Core`，任务文件夹为空时删除文件夹。
- `QueryStartupTask`：返回 `ready|missing|mismatch|error`，不返回用户 SID 或凭据。
- `RunStartupTask`：通过 `IRegisteredTask::Run` 启动任务，确保 Core 使用任务定义的非提升交互用户上下文。

内部 C++ 接口：

```cpp
TaskResult install_startup_task(const std::filesystem::path& install_root);
TaskResult remove_startup_task();
TaskStatus query_startup_task(const std::filesystem::path& install_root);
```

实现使用 `ITaskService`、`ITaskFolder`、`ITaskDefinition`、`ILogonTrigger`、`IExecAction`、`ITaskSettings` 和 `ITaskSettings2`。COM/BSTR/VARIANT 使用 RAII，错误只映射为稳定错误码，不把完整任务 XML 或本机身份写入日志。

## 5. NSIS 集成

安装流程在 Core 文件和配置写入后、首次启动前调用 `InstallStartupTask`。任务注册失败属于安装失败，显示中文错误并允许重试。

成功注册任务后：

- 删除遗留 `HKCU\Software\Microsoft\Windows\CurrentVersion\Run\AetherisCore`，避免双启动。
- 删除遗留 Startup 文件夹中的 `Aetheris Core.lnk`。
- 安装器通过 `RunStartupTask` 首次启动 Core并验证状态，不从提升后的 Setup 直接启动 Core；计划任务负责后续登录和故障恢复。

升级安装调用同一接口覆盖任务定义，因此路径、版本或安装目录变化后能够自动修复。升级不因任务已存在而失败。

卸载流程先调用 `RemoveStartupTask`，再正常退出 Core、撤销 device credential 并删除程序文件。任务删除失败时显示警告并记录非敏感错误码，不能静默留下指向已删除程序的任务。

## 6. 生命周期诊断

Core 在 `logs\core.log` 增加：

- `lifecycle start`：版本、PID、是否 supervised。
- `lifecycle stop reason=user_exit`：托盘或本地 Lens 正常退出。
- `lifecycle fatal error=<ExceptionType>`：顶层异常退出。

日志不记录命令正文、事件 payload、token、enrollment code 或 OCR 原文。日志达到 1 MB 时轮转，最多保留 5 个文件。

突然断电、Task Manager 强制结束或 native crash 可能没有 stop 日志；这时结合 core-status 最后更新时间和 Task Scheduler LastTaskResult 判断。

## 7. 当前安装修复

0.4.6 构建验证后，使用原生 COM 注册逻辑修复当前 `D:\FbBrowser\Aetheris` 安装：

1. 保留现有 config、DPAPI credential、SQLite 和项目授权。
2. 更新 `AetherisCore.exe` 到已验证的 0.4.6。
3. 注册 `\Aetheris\Aetheris Core`。
4. 删除遗留 Run/Startup 项。
5. 启动 Core 并确认 `registered/running`。
6. 执行一次受控异常退出测试，确认 1 分钟内自动恢复。
7. 托盘正常退出测试确认不自动恢复，随后通过任务手工启动恢复运行。

当前安装修复不得重新 enrollment，不替换或输出 device credential。

## 8. 测试与验收

自动化测试：

- C++ 任务定义包含登录触发、InteractiveToken、10 秒延迟、1 分钟重启和 255 次重试。
- Action executable、arguments 和 working directory 分离且正确引用 Unicode/空格路径。
- 安装、重复安装、路径变更、查询和删除任务。
- NSIS 不再写 HKCU Run，安装调用 InstallStartupTask，卸载调用 RemoveStartupTask。
- Core 正常退出码为 0，顶层异常退出码为 1。
- 生命周期日志轮转和敏感字段排除。

本机验收：

- 安装后任务存在且状态 ready。
- Core 已运行时再次触发任务不会产生第二个逻辑实例。
- 强制结束 Core 后 1 分钟内产生新 PID，并恢复 registered/running。
- 托盘正常退出后等待 2 分钟不自动重启。
- 手工运行任务恢复 Core。
- 注销并重新登录后 Core 自动启动；该项记录为用户可执行验收步骤，不在当前会话中自动注销用户。
- 进程树不存在 cmd、PowerShell、`schtasks.exe` 或额外常驻 watchdog。
- `/downloads/client` 返回 `AetherisSetup-0.4.6.exe`，完整哈希一致。
