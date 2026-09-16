# Aetheris Core Supervision Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 Aetheris Core 0.4.6 实现用户登录自启动、异常退出自动重启、正常退出不重启，并修复当前正式安装。

**Architecture:** `AetherisProvisioning.dll` 使用 Task Scheduler 2.0 COM API 注册用户级监督任务；NSIS 安装/升级负责创建或修复任务并移除旧 Run/Startup 项；Core 通过退出码和生命周期日志区分正常退出与异常失败。当前安装通过同一原生 COM 逻辑原地修复，不重新 enrollment。

**Tech Stack:** C++17/MSVC Win32、Task Scheduler 2.0 COM、NSIS 3.12 Unicode/MUI2、Python 3.11+、PyInstaller windowed、Windows Task Scheduler。

**Spec:** `docs/superpowers/specs/2026-09-07-core-supervision-design.md`

## Global Constraints

- 发布版本统一为 0.4.6。
- 任务路径固定为 `\Aetheris\Aetheris Core`。
- 登录触发延迟 10 秒，异常退出重启间隔 1 分钟，重试 255 次。
- 托盘正常退出码为 0，顶层异常退出码为 1。
- 不调用 cmd、PowerShell 或 `schtasks.exe`。
- 安装/修复允许一次 UAC，仅用于注册任务和维护安装文件；Core 任务保持 LUA。
- 不修改或输出当前 device/control credential。
- 用户未登录时不在 Session 0 启动托盘。
- 工作区不是 Git 仓库，以测试、哈希和部署记录作为检查点。

---

### Task 1: Core 退出语义与生命周期日志

**Files:**
- Create: `src/aetheris/lifecycle.py`
- Modify: `src/aetheris/tray.py`
- Create: `tests/test_lifecycle.py`

**Interfaces:**
- `LifecycleLog(path, max_bytes=1048576, backups=5)`。
- `LifecycleLog.start(supervised: bool)`。
- `LifecycleLog.stop(reason: str)`。
- `LifecycleLog.fatal(error_type: str)`。
- `main() -> int` 或等价入口，正常退出 0、顶层异常 1。

- [x] 写失败测试，断言 start/user_exit/fatal 记录、1 MB/5 文件轮转和敏感文本不进入日志。
- [x] 写失败测试，断言 `--supervised` 可解析，正常托盘退出为 0，顶层异常为 1。
- [x] 运行 `python -m unittest tests.test_lifecycle -v` 确认缺少实现。
- [x] 实现最小生命周期日志与退出码，不改变采集循环行为。
- [x] 运行 lifecycle、tray、single-instance 测试。

### Task 2: Task Scheduler COM 原生实现

**Files:**
- Create: `native/provisioning/include/aetheris/startup_task.hpp`
- Create: `native/provisioning/src/startup_task.cpp`
- Modify: `native/provisioning/src/plugin.cpp`
- Modify: `native/provisioning/CMakeLists.txt`
- Modify: `native/provisioning/tests/provisioning_tests.cpp`
- Modify: `native/provisioning/src/exports.def`

**Interfaces:**
- `TaskResult install_startup_task(const std::filesystem::path&)`。
- `TaskResult remove_startup_task()`。
- `TaskStatus query_startup_task(const std::filesystem::path&)`。
- NSIS exports: `InstallStartupTask`, `RemoveStartupTask`, `QueryStartupTask`。

- [ ] 写失败测试，使用测试任务路径验证安装、重复覆盖、Unicode/空格路径、查询和删除。
- [ ] 运行 C++ 测试，确认缺少 startup task 实现。
- [ ] 使用 `ITaskService`/`ITaskDefinition` 实现 InteractiveToken LogonTrigger、10 秒延迟、1 分钟/255 重启和 IgnoreNew。
- [ ] Action 分别设置 executable、arguments、working directory，不构造 shell 命令。
- [ ] 构建 Win32 DLL，检查三个稳定导出并运行 ctest。

### Task 3: NSIS 安装、升级和卸载集成

**Files:**
- Modify: `installer/windows/nsis/AetherisSetup.nsi`
- Modify: `tests/test_nsis_script.py`
- Modify: `tests/test_native_plugin.py`

**Interfaces:**
- Setup 在首次启动 Core 前调用 `InstallStartupTask`。
- Uninstall 在删除 Core 前调用 `RemoveStartupTask`。

- [ ] 写失败测试，真实编译 NSIS 并验证不再写 HKCU Run、包含 Install/RemoveStartupTask 调用。
- [ ] 实现安装/升级覆盖任务，删除遗留 Run value 和 Startup shortcut。
- [ ] 实现卸载任务删除；失败显示非敏感警告，不静默遗留任务。
- [ ] 用测试安装目录编译并运行 NSIS 安装/卸载，查询任务 ready/missing。
- [ ] 验证 Setup/Core/任务进程树不含 cmd、PowerShell 或 `schtasks.exe`。

### Task 4: 0.4.6 构建与服务器发布

**Files:**
- Modify: `src/aetheris/version.py`
- Modify: `pyproject.toml`
- Modify: `README.md`
- Modify: `installer/windows/README_INSTALL.md`
- Modify: `deploy/.env.example`
- Modify: `docs/operations/implementation-log.md`

**Interfaces:**
- `AetherisCore-0.4.6.exe`。
- `AetherisSetup-0.4.6.exe`。
- `/downloads/client` 返回 0.4.6 Setup。

- [ ] 更新版本夹具并运行完整 Python、C++、Go、Admin Web 和 Model Gateway 测试。
- [ ] 构建 Core、原生插件、NSIS Setup、checksum 和 Linux Go Server。
- [ ] 本机验证 NSIS 可见窗口、单进程、无 shell 子进程和任务注册。
- [ ] 上传到 `/opt/aetheris/staging/0.4.6`，校验哈希后原子切换并保留 0.4.5 回滚文件。
- [ ] 验证 health/admin/download 200、完整 HTTP 哈希、二进制不含 enrollment secret 和磁盘低于 100 GB。

### Task 5: 当前安装修复与自动恢复验收

**Files:**
- Modify in place: `D:\FbBrowser\Aetheris\AetherisCore.exe`
- Preserve: `D:\FbBrowser\Aetheris\config\*`
- Preserve: `D:\FbBrowser\Aetheris\data\*`

**Interfaces:**
- 当前任务状态 `ready`。
- 当前 Core 状态 `registered/running`。

- [ ] 备份当前 0.4.4 Core 和非敏感配置元数据，不复制/输出 credential 内容。
- [ ] 停止当前 Core，原子替换为已验证的 0.4.6，注册监督任务并删除遗留启动项。
- [ ] 触发任务并验证单实例、registered/running 和 16 个项目。
- [ ] 受控强制终止 Core，等待最多 90 秒，验证新 PID 自动出现并恢复 registered/running。
- [ ] 通过本地 Lens 正常退出，等待 120 秒确认不自动重启；随后手工运行任务恢复 Core。
- [ ] 记录注销/重新登录自启动为用户验收步骤，不在当前会话注销用户。
