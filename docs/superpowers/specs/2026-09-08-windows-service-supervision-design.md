# Aetheris Core Windows 服务监管设计

**日期：** 2026-09-08  
**状态：** 已确认并实施，本机服务镜像访问问题排查中  
**目标版本：** 0.4.8  
**范围：** Windows 服务监管、活动控制台用户中的 Core/托盘启动、服务与 Core IPC、异常恢复、正常退出、NSIS 安装升级回滚及本机验收。

## 1. 决策与目标

0.4.7 的 Task Scheduler 方案在目标机器上被终端策略阻止。原生 COM 注册返回 `0x80004005`，提权安装器通过系统 `schtasks.exe` 导入 XML 仍返回 `0x80070005`。继续依赖计划任务无法形成可靠安装链路。

0.4.8 改为 Windows 服务监管：

- Windows 启动后服务自动运行。
- 当前活动控制台用户登录或解锁后，在该用户会话中启动 Core。
- Core 异常退出后自动恢复，托盘主动退出后本次登录会话不恢复。
- 托盘、窗口采集、浏览器 OCR、本地 Lens 和 DPAPI 始终运行在交互用户身份下。
- 服务不读取采集正文、截图、SQLite、设备 token 或控制 token。
- 安装和运行过程不调用 `cmd.exe`、PowerShell、`sc.exe` 或 `schtasks.exe`。
- 首期只监管当前活动控制台用户，不支持并行 RDP 会话。
- 本机 Beta 测试允许使用已安装到 LocalMachine Root 与 TrustedPublisher、具有代码签名 EKU 且通过 `signtool verify /pa` 的测试证书。`/kp` 用于内核模式驱动签名验证，不作为用户态 `SERVICE_WIN32_OWN_PROCESS` 的验收条件。对外分发仍应使用正式 Authenticode 证书。

原计划任务规格 `2026-09-07-core-supervision-design.md` 被本规格替代。计划任务代码和 repair 产物不进入 0.4.8 发布包。

## 2. 组件与信任边界

### 2.1 Aetheris Core Service

`AetherisCoreService.exe` 是原生 C++ Windows 服务，使用 LocalSystem 账户和 Automatic Delayed Start。它只负责：

- 监听 SCM 启停和会话变化通知。
- 识别当前活动控制台会话。
- 使用该会话的用户令牌启动并监管 Core。
- 维护 Core 进程句柄、恢复退避和正常退出抑制状态。
- 提供最小、受 ACL 保护的状态 IPC。

服务不加载 Python、不初始化采集适配器、不访问网络，不解析 AetherisEvent，也不打开 credential、SQLite、OCR 临时图像或项目文件。

### 2.2 Aetheris Core User Agent

`AetherisCore.exe` 保持用户态采集引擎和托盘应用。服务使用 `WTSQueryUserToken`、`DuplicateTokenEx`、`CreateEnvironmentBlock` 和 `CreateProcessAsUserW` 将它启动到活动控制台会话，参数固定为：

```text
--config "<用户安装目录>\config\aetheris.json" --supervised --service-session <session-id>
```

Core 子进程使用登录用户的非提升令牌，并将 `STARTUPINFO.lpDesktop` 固定为 `winsta0\\default`，因此 DPAPI CurrentUser、窗口枚举、截图、IDE 会话数据库、托盘和本地 Lens 都处于正确交互桌面。服务绝不把自己的 LocalSystem 令牌传给 Core，也不接受外部输入覆盖桌面名。

### 2.3 安装布局

LocalSystem 服务二进制不能放在普通用户可修改的自选目录，否则会形成本地提权路径。固定布局为：

```text
%ProgramFiles%\Aetheris\Service\
  AetherisCoreService.exe

%ProgramData%\Aetheris\Service\
  state.json
  service.log

<用户选择的安装目录>\
  AetherisCore.exe
  Uninstall.exe
  config\
  data\
  logs\
```

服务目录和 HKLM 服务配置仅允许 SYSTEM 与 Administrators 修改。用户安装目录可由当前用户管理，但 Core 始终以该用户身份运行，所以替换用户目录中的文件不会获得 LocalSystem 权限。

HKLM `Software\Aetheris\Core` 保存规范化的 Core 安装路径、预期版本和安装用户 SID。服务只接受由提升安装器写入的该路径，不接受 IPC 客户端提供可执行路径或参数。

## 3. 服务与会话生命周期

服务注册名固定为 `AetherisCoreService`，显示名为 `Aetheris Core Service`。服务入口使用 `StartServiceCtrlDispatcherW`，控制处理器使用 `RegisterServiceCtrlHandlerExW` 并接受 `SERVICE_CONTROL_SESSIONCHANGE`。

启动流程：

1. 校验 HKLM 配置和 Core 绝对路径。
2. 创建全局服务单实例互斥量。
3. 创建状态 IPC 和只读诊断状态。
4. 调用 `WTSGetActiveConsoleSessionId`。
5. 如果没有活动用户，保持 Running 并等待会话事件。
6. 如果存在活动控制台用户，校验该用户 SID 与安装用户 SID，然后启动 Core。
7. 等待 Core 的 authenticated `ready` 消息；60 秒未 ready 视为启动失败。

会话事件行为：

- Logon/Unlock/ConsoleConnect：重新识别活动控制台用户并在需要时启动 Core。
- ConsoleDisconnect：不立即停止 Core，避免短暂锁屏或显示切换造成采集中断。
- Logoff：请求对应 Core 正常停止，清除该登录会话的退出抑制状态。
- Fast User Switching：通过 `session_stop` 请求旧活动会话 Core 退出，不设置用户退出抑制；再为新的活动控制台会话启动一个实例。
- RDP 会话：首期忽略，不启动并行 Core。

Core 继续使用现有本地单实例互斥量。服务检测到已存在且身份、session ID 和安装路径匹配的 Core 时接管观察，不创建第二个实例；无法验证的同名进程不被服务终止。

## 4. IPC 协议

命名管道固定为：

```text
\\.\pipe\Aetheris.Core.Service.v1
```

服务创建管道时使用显式安全描述符，只允许 SYSTEM、Administrators 和当前安装用户 SID 访问。连接后通过 `GetNamedPipeClientProcessId`、进程令牌 SID 和 SessionId 三重校验客户端；不匹配立即断开。

协议使用最大 16 KiB 的长度前缀 UTF-8 JSON。每条消息包含 `version=1`、`type`、`pid`、`session_id` 和单调时间。允许的消息类型只有：

- `ready`：Core 已完成配置、凭据和本地状态初始化。
- `heartbeat`：Core 仍响应，不含采集统计或正文。
- `normal_exit`：用户通过托盘或受保护本地 Lens 主动退出。
- `resume`：用户从开始菜单手工启动 Core，请求清除当前会话的退出抑制并由服务接管监管。
- `status_request` / `status_response`：Lens 查询服务和恢复状态。
- `prepare_update` / `update_ready`：安装器升级时请求 Core 安全停止。
- `session_stop`：活动控制台用户切换或注销时，由服务请求旧会话 Core 安全停止。

协议不接受任意命令、路径、环境变量或 shell 参数，不传递 enrollment code、device token、control token、AetherisEvent、OCR 文本或项目内容。

只有同时满足以下条件才认定为正常退出：Core 先发送通过身份校验的 `normal_exit`，并在 10 秒内以退出码 0 结束。仅出现退出码 0、管道断开或进程消失都按异常处理。

## 5. 自动恢复与退出语义

Core 异常退出后等待 60 秒再启动。服务为当前登录会话保存最近 10 分钟的失败时间戳：

- 少于 5 次：按 60 秒间隔恢复。
- 达到 5 次：进入 15 分钟退避，状态为 `crash_loop_backoff`。
- Core 连续健康运行 30 分钟后清空失败计数。

用户主动退出后，服务状态为 `user_suppressed`，在当前登录会话内不再启动 Core。该状态写入 `%ProgramData%\Aetheris\Service\state.json`，内容只有用户 SID、SessionId、当前 Windows 启动周期标识和退出时间，不含采集数据或凭据。服务自身重启时继续遵守抑制；用户注销或 Windows 重启后清除。

安装器创建“Aetheris Core”开始菜单快捷方式。用户在 `user_suppressed` 状态下手工运行 Core 时，Core 以当前用户身份连接服务并发送 `resume`；服务校验 PID/SID/SessionId 后清除抑制、接管该进程并等待 `ready`。服务自身不会接受 IPC 客户端提供的启动路径。

SCM 为服务自身设置恢复动作：第一次、第二次和后续异常退出均在 60 秒后重启服务，失败计数 24 小时后重置。管理员明确停止服务时不触发恢复。

## 6. 日志与可观察性

服务日志写入 `%ProgramData%\Aetheris\Service\service.log`，单文件 1 MiB，最多保留 5 个。允许记录版本、服务状态、SessionId、Core PID、退出码、恢复次数和稳定错误码。

禁止记录用户名、SID 全文、可执行命令行、环境块、配置正文、token、enrollment code、项目路径、窗口标题、URL、OCR 文本或事件 payload。SID 只可在需要关联时记录不可逆短哈希。

本地 Lens 展示：服务状态、Core 状态、启动时间、最近心跳、最近退出类型、下次恢复时间和退避原因。Lens 不提供任意服务命令；用户可执行的动作限定为“启动 Core”和“退出 Core”。

## 7. 安装、升级与回滚

NSIS 仍是唯一可见安装程序，仅请求一次 UAC。`AetherisProvisioning.dll` 使用原生 SCM API：`OpenSCManagerW`、`CreateServiceW`、`ChangeServiceConfigW`、`ChangeServiceConfig2W`、`StartServiceW`、`ControlService` 和 `DeleteService`。禁止调用 shell 或系统命令行工具。

安装顺序：

1. 将服务与 Core payload 解压到安装 staging。
2. 完成哈希校验。
3. 若现有 `aetheris.json` 和 DPAPI device credential 可完成 heartbeat，则复用身份并跳过 enrollment。
4. 仅在 credential 缺失、损坏或被服务端拒绝时显示 enrollment 页面。
5. 原子提交 Core、配置和服务二进制。
6. 写入受保护 HKLM 配置。
7. 创建或更新服务，设置 Delayed Auto Start 和 SCM Recovery。
8. 启动服务，等待服务 Running、Core `ready` 和匹配设备状态。
9. 全部通过后才显示安装完成。

升级时先通过 IPC 请求 Core `prepare_update`，再停止服务。旧服务二进制、Core 和 HKLM 配置保存为同卷 `.previous`。新服务或 Core 未在规定时间内 ready 时：停止新服务、恢复旧二进制和 SCM/HKLM 配置、启动旧服务，并保留 config、credential、data 和项目授权。

首次安装失败时删除本次创建的服务和 staging，但不递归删除用户目录中的未知文件。若 bootstrap 已签发 device credential，则保留加密 credential 供同版本重试，绝不写明文 token。

当前 0.4.7 安装已恢复 `aetheris.json`、device/control credential 和 Core payload，但 credential 尚待 heartbeat 验证，且没有启动监管器、Core 未运行。0.4.8 安装器必须先验证并尽可能复用现有身份；只有验证失败才要求 enrollment，随后迁移到 Windows 服务并删除计划任务 repair 产物。

## 8. 卸载

卸载顺序：请求 Core 正常退出、停止服务、撤销设备凭据、删除服务注册、删除服务二进制和程序文件。删除服务失败时不能继续删除服务二进制，避免留下损坏的 SCM 路径。

默认保留不含有效 credential 的 `aetheris.json`、SQLite、日志和项目授权。卸载 UI 提供“同时删除本地数据与项目设置”复选框，默认不选；无论选择什么都不递归删除安装目录中的未知文件。

卸载后删除 `device.credential` 和 `control.credential`。服务端不可达时记录 `revocation_pending`，本地 credential 仍立即删除。

## 9. 测试与验收

自动化测试：

- 服务命令行、受保护安装布局和 HKLM 配置解析。
- SCM install/update/query/start/stop/remove 的稳定结果码和幂等性。
- LocalSystem 服务只以活动控制台用户令牌启动 Core。
- 非活动用户、RDP 和 SID 不匹配时不启动。
- 命名管道 ACL、客户端 PID/SID/SessionId 校验、16 KiB 限制和未知消息拒绝。
- authenticated normal exit、异常退出、60 秒恢复、5/10 分钟退避和 30 分钟重置。
- 服务重启后保留正常退出抑制，注销或重启后清除。
- NSIS 编译和静态检查确认不引用 cmd、PowerShell、`sc.exe`、`schtasks.exe` 或计划任务插件接口。
- 升级失败恢复旧服务/Core，config、credential、SQLite 和项目授权保持字节一致。
- 服务日志和 IPC 不包含敏感字段。

本机验收：

1. 单次 UAC 完成 0.4.8 安装，复用当前设备身份。
2. `AetherisCoreService` 为 Running、Automatic Delayed Start，并配置 SCM Recovery。
3. Core 在当前用户会话中非提升运行，托盘可见，本地 Lens 可访问。
4. 进程树为 Service 到用户态 Core，不出现 cmd、PowerShell、`sc.exe` 或 `schtasks.exe`。
5. 强制结束 Core 后 90 秒内出现新 PID并恢复 `registered/running`。
6. 托盘主动退出后等待 120 秒不重启；服务重启后仍不重启。
7. 重新登录或重启 Windows 后 Core 自动启动；该步骤由用户执行，不在自动化会话中强制注销或重启。
8. Gateway heartbeat 和事件 ingest 成功，服务端设备列表保持同一稳定 device ID。
9. 发布下载的 Setup 哈希与本机构建一致，服务器总磁盘占用仍小于 100 GB。

## 10. 后续范围

企业版可在独立规格中增加并行 RDP/VDI 会话、多用户每会话 Core 和企业证书轮换。本期不实现多会话聚合，也不把采集逻辑迁入 LocalSystem 服务。
