# Aetheris NSIS Windows 安装器设计

**日期：** 2026-09-07  
**状态：** 已实施并部署  
**目标版本：** 0.4.1  
**范围：** NSIS 安装器、原生 provisioning 插件、首次设备注册、零项目安装、动态项目管理、升级与卸载。

## 1. 背景与目标

0.4.0 Setup 使用 PyInstaller 打包 Python/Tk 界面，通过多个模态对话框依次收集安装目录、项目、用户标识和 enrollment code。隐藏根窗口与 transient 子窗口组合会产生不可见模态窗口，Setup 进程继续运行但安装状态机没有进入 Core 复制阶段。

0.4.1 使用真正的 NSIS MUI2 安装器替换 Python Setup。安装器必须始终保持一个可见主窗口，提供标准 Windows 安装体验，并在安装完成后退出，只保留托盘 `AetherisCore.exe`。安装和卸载过程不得调用 cmd、PowerShell 或 Python Setup 进程。

项目不是安装前置条件。用户可以在首次安装时添加项目，也可以跳过并在安装后的本地 Lens 中动态添加。

## 2. 交付物与组件边界

发布产物：

- `AetherisSetup-0.4.1.exe`：NSIS MUI2 单文件安装器，是唯一公开安装入口。
- `AetherisCore-0.4.1.exe`：被 Setup 内嵌，安装后稳定命名为 `AetherisCore.exe`。
- `AetherisProvisioning.dll`：Win32 Unicode NSIS 原生插件，在安装器进程内执行项目扫描、设备注册、DPAPI 和配置写入。
- `Uninstall.exe`：由 NSIS 生成的原生卸载器。

代码边界：

```text
installer/windows/nsis/AetherisSetup.nsi
installer/windows/nsis/pages/*.nsh
native/provisioning/CMakeLists.txt
native/provisioning/src/*.cpp
native/provisioning/tests/*
src/aetheris/project_registry.py
src/aetheris/local_view.py
src/aetheris/tray.py
scripts/build_windows_core.py
scripts/build_windows_setup.py
```

NSIS 负责可见向导、页面状态、文件安装、当前用户启动项、完成页和卸载。原生插件负责不能安全地用 NSIS 脚本表达的操作。Core 不再包含首次安装向导，只负责托盘、状态页、采集、动态项目配置和上传。

## 3. 安装向导

向导使用 MUI2 和 Unicode 构建，固定为单主窗口，页面顺序如下：

1. 欢迎页。
2. 安装目录页。
3. 可选项目页。
4. 用户与设备注册页。
5. 安装、注册和验证进度页。
6. 完成页。

### 3.1 安装目录

默认目录为 `$LOCALAPPDATA\Aetheris`。用户可以直接编辑或使用浏览按钮选择目录。安装为当前用户范围，不请求管理员权限，不写入 `Program Files`。

继续前检查：

- 目录可以创建或写入。
- 路径不是文件。
- 可用空间不少于 Core、临时 staging 和 512 MB 本地队列上限所需的安全余量。
- 目录中已有未知文件时不删除、不覆盖未知文件。

### 3.2 可选项目

项目页提供：

- “扫描目录”输入框和浏览按钮。
- “扫描项目”按钮。
- Git/SVN 项目多选列表。
- “稍后添加项目”选项。

扫描最多深入 8 层、返回 100 个项目，不跟随符号链接、junction 或 reparse point，跳过 `.git`、`.svn`、`node_modules`、虚拟环境、构建输出和缓存目录。扫描在原生工作线程执行，页面显示进度并保持响应。

未选择目录、扫描失败、没有发现项目或用户选择稍后添加，都不阻止安装。零项目配置是合法状态。

### 3.3 用户与注册

页面显示：

- 用户标识，默认 `DOMAIN\USERNAME`，允许修改。
- enrollment code 密码输入框。
- Gateway 只读状态，不提供手工编辑。生产构建固定为下载来源对应的服务器地址。

用户标识和 enrollment code 必须填写。错误 enrollment 或暂时网络失败停留在当前流程并允许重试，不清除已填写的安装目录、项目和用户标识。

### 3.4 进度与完成

进度阶段固定为：

```text
prepare_staging
extract_core
bootstrap_device
protect_credential
verify_heartbeat
write_configuration
write_startup
launch_core
verify_core_status
commit_installation
```

页面显示当前阶段和可操作的中文错误，不显示凭据或请求正文。完成页只有在设备注册、凭据加密、heartbeat 和 Core 启动验证全部通过后出现。

完成后 Setup 退出，正常运行状态只保留 `AetherisCore.exe`。PyInstaller one-file Core 的两个同名父子进程视为一个逻辑 Core 实例。

## 4. 原生 Provisioning 插件

插件使用 MSVC 构建为 Win32 Unicode DLL，与 NSIS 进程位数匹配。链接 Windows 系统库 `WinHTTP`、`Crypt32`、`Bcrypt`、`Shell32` 和 `Advapi32`，不引入需要单独安装的运行时。

插件公开给 NSIS 的 ABI 只返回非敏感状态：

- `StartProjectScan`：异步开始项目扫描。
- `PollProjectScan`：返回进度、非敏感错误码和项目列表句柄。
- `PopulateProjectList`：把扫描结果写入 NSIS 页面 ListView。
- `ProvisionDevice`：在单次调用内部完成 bootstrap、token DPAPI 加密和 heartbeat。
- `WriteConfiguration`：原子写入配置和项目列表。
- `VerifyCoreStatus`：验证 Core 版本、设备 ID 和注册状态。
- `RevokeDevice`：卸载时撤销当前设备凭据，只返回非敏感结果。

`ProvisionDevice` 不把 device token 返回到 NSIS 插件栈。bootstrap 响应中的 token 在插件内存中立即交给 DPAPI CurrentUser，加密结果写入 staging credential 后清零临时缓冲区。NSIS 只接收 tenant ID、subject ID、device ID、状态码和错误类别。

项目列表可以进入安装配置，但 enrollment code、device token 和 bootstrap 原始响应不得写入命令行、环境变量、临时文件、NSIS detail log 或应用日志。

## 5. 安装布局与原子提交

正式布局：

```text
<安装目录>\
  AetherisCore.exe
  Uninstall.exe
  config\aetheris.json
  config\device.credential
  config\control.credential
  data\client.db
  data\core-status.json
  logs\core.log
  logs\setup.log
```

安装先写入 `<安装目录>\.installing\<随机标识>\`。只有所有强制步骤通过后，才把 Setup 拥有的文件原子移动到正式位置并写入启动项。

升级时保留 `data`、已有加密 credential 和用户项目授权。发现运行中的 Core 时，安装器显示退出提示并允许重新检测，不强制结束进程。0.3.x 明文 token 不迁移为 device token，必须重新 enrollment。

## 6. 失败与回滚

项目相关情况不是安装失败：

- 项目扫描错误只显示警告。
- 零项目允许安装成功。
- 不可访问项目不写入授权列表。
- Core 在零项目时显示 `waiting_for_project`，不是 `error`。

阻止安装成功的情况：

- Core payload 损坏或无法写入。
- enrollment code 无效。
- DPAPI 加密、写入或解密回读失败。
- bootstrap/heartbeat 返回的 tenant、subject 或 device 身份不一致。
- 配置或当前用户启动项无法写入。
- Core 无法启动或未在规定时间内写出匹配版本和设备身份的状态。

网络超时和 5xx 允许重试。401/403 显示 enrollment 无效。取消或失败时删除本次 staging 和本次创建的启动项，保留诊断日志，不删除安装目录中的未知文件。已经签发但尚未提交安装的设备凭据由服务端过期策略处理；客户端不把 token 留在明文介质中。

## 7. 动态项目管理

首次安装后的项目管理位于本地 Lens，并从托盘“管理项目”进入。接口只绑定 `127.0.0.1`。

支持操作：

- 添加扫描根目录。
- 扫描并批量授权 Git/SVN 项目。
- 手动添加没有项目 marker 的目录，状态为 `pending_classification`。
- 暂停、恢复、重新扫描和移除项目。
- 查看项目 VCS、授权状态、adapter 状态和最近采集时间。

项目变化原子写入 `config\aetheris.json`。配置包含递增 `project_revision`。Core 的 `ProjectRegistry` 监控文件变化，校验路径仍在本机且可访问，然后按差异创建或释放 Git、SVN、Visual Studio 和项目关联适配器，无需重启。

移除项目立即停止新采集，但不自动删除历史事件。历史删除是单独操作并要求再次确认。每次添加、授权、暂停、恢复和移除都生成本地审计事件。

本地 Lens 使用安装时生成的随机控制令牌，单独保存在 DPAPI 保护的 `config\control.credential` 中，不与 device token 共用文件或格式。状态读取使用同源页面；修改请求要求控制令牌、POST、Origin/Host 校验，拒绝非 loopback 访问。

## 8. 卸载

卸载由 NSIS `Uninstall.exe` 完成，不生成或调用 PowerShell 脚本。卸载器请求 Core 通过本地控制接口正常退出并等待进程结束，然后调用设备撤销接口、删除 `device.credential`、`control.credential`、程序文件和当前用户启动项。服务端不可达时记录 `revocation_pending`，删除本地 credential，并在 Admin 中保留可审计的离线设备撤销操作。

默认保留不含 secret 的 `aetheris.json`、`data`、日志和项目授权，便于重新安装时恢复项目选择；有效 credential 永不保留。卸载页提供明确的“同时删除本地数据与项目设置”复选框，默认不选。无论用户选择什么，都不递归删除整个安装目录中的未知文件。

## 9. 构建与发布

版本唯一来源为 `src/aetheris/version.py`，本次发布值为 `0.4.1`。构建使用：

- NSIS 3.x Unicode 与 MUI2。
- MSVC x86 工具链构建 provisioning 插件。
- PyInstaller `--onefile --windowed` 构建 Core。

顺序：

1. 运行 Python、Go、Admin Web 和原生插件测试。
2. 构建 `AetherisProvisioning.dll`。
3. 构建 `AetherisCore-0.4.1.exe`。
4. 使用 `makensis` 内嵌 DLL 和 Core，生成 `AetherisSetup-0.4.1.exe`。
5. 生成 SHA-256，扫描产物确认不包含真实 enrollment secret、device token 或服务器密码。
6. 本机执行 UI 自动化和安装/卸载验收。
7. 上传 Setup、Core 和 checksum，原子切换 `/downloads/client`。
8. 验证完整 HTTP 下载哈希后再宣布发布成功。

生产 Gateway URL 通过构建定义注入。测试构建可以注入 loopback fake Gateway；生产安装器不提供运行时 Gateway 输入框或命令行覆盖。

## 10. 测试与验收

自动化测试：

- 原生插件项目扫描的深度、数量、排除目录、junction 和 Unicode 路径。
- bootstrap JSON、WinHTTP 状态映射、DPAPI round-trip、损坏 credential 和身份不一致。
- enrollment/token 不进入日志、命令行、环境变量或明文文件。
- NSIS 脚本构建、payload 哈希、当前用户安装和原生卸载。
- ProjectRegistry 的原子更新、版本冲突、热添加、暂停、恢复和移除。
- Local Lens 的 loopback、Host、Origin、控制令牌和项目修改接口。

本机 UI 自动化：

- Setup 启动后存在一个可见主窗口。
- 安装目录可编辑和浏览。
- 项目页可跳过，零项目安装成功。
- 扫描过程显示进度且窗口响应。
- 错误 enrollment 不进入完成页。
- 网络失败可重试且保留输入。
- 成功安装后 Setup 退出并启动 Core。
- 进程树没有 cmd、PowerShell 或 Python Setup。
- 本地 Lens 动态添加项目后 Core 不重启即开始相应 adapter。
- `Uninstall.exe` 正常退出 Core，默认保留数据，选择后才删除 Aetheris 数据。

服务端实机验收：

- 错误 enrollment 返回 401 且不创建设备。
- 正确 bootstrap、heartbeat 和 ingest 成功。
- Admin 设备列表显示主体和设备。
- 重复事件返回 duplicate。
- `/downloads/client` 返回 `AetherisSetup-0.4.1.exe`，内容长度与 SHA-256 一致。
- `/opt/aetheris` 总占用继续低于 100 GB。

## 11. 明确不采用

- 不再发布 Python/Tk Setup 作为默认安装器。
- 不使用 IExpress、CAB、批处理、cmd 或 PowerShell 完成安装。
- 不把 enrollment code 或 device token 放入 helper EXE 命令行。
- 不要求首次安装必须选择项目。
- 不在项目扫描期间隐藏安装窗口或阻塞 UI 消息循环。
