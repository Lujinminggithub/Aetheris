# Aetheris NSIS Windows Installer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 发布真正的 NSIS 0.4.1 安装器，提供始终可见的单窗口向导、安全设备注册、可选首次项目授权、安装后动态项目管理和原生卸载。

**Architecture:** NSIS MUI2 负责全部可见安装页面和文件生命周期；Win32 Unicode `AetherisProvisioning.dll` 在 NSIS 进程内完成项目扫描、WinHTTP bootstrap/heartbeat、DPAPI 和配置写入；Core/Lens 通过版本化 ProjectRegistry 热加载动态项目。Python/Tk Setup 不再作为生产入口。

**Tech Stack:** NSIS 3.x Unicode、MUI2/nsDialogs、C++17/MSVC Win32、WinHTTP/Crypt32/Bcrypt、Python 3.11+、PyInstaller windowed、SQLite、Go Server。

**Spec:** `docs/superpowers/specs/2026-09-07-nsis-windows-installer-design.md`

## Global Constraints

- 发布版本统一为 `0.4.1`，唯一来源是 `src/aetheris/version.py`。
- Setup、卸载和 provisioning 不调用 cmd、PowerShell 或 Python Setup。
- enrollment code 和 device token 不进入命令行、环境变量、临时文件或日志。
- 项目是可选项；零项目必须可以成功安装并启动 Core。
- Setup 只安装到当前用户可写目录，不请求管理员权限。
- 本地 Lens 只监听 `127.0.0.1`，修改请求要求控制令牌、Host 和 Origin 校验。
- `/opt/aetheris` 总占用必须小于 100 GB。
- 工作区没有 Git 元数据，各任务以测试和实施日志作为检查点，不执行 commit。

---

### Task 1: 版本与 NSIS 构建契约

**Files:**
- Modify: `src/aetheris/version.py`
- Modify: `pyproject.toml`
- Replace: `scripts/build_windows_setup.py`
- Create: `tests/test_nsis_build.py`

**Interfaces:**
- Consumes: `AetherisCore-{__version__}.exe`。
- Produces: `build_nsis(core: Path, output_dir: Path) -> Path`，返回 `AetherisSetup-0.4.1.exe`。

- [x] 写失败测试，断言版本为 0.4.1、构建器调用 `makensis`、传入 Core、插件、Gateway 定义并且不调用 PyInstaller Setup。
- [x] 运行 `python -m unittest tests.test_nsis_build -v`，确认因缺少 NSIS 构建接口失败。
- [x] 实现 `makensis` 路径发现、唯一临时目录、构建定义、退出码和产物校验。
- [x] 安装或定位 NSIS 3.x Unicode 工具链，并验证 `makensis /VERSION`。
- [x] 运行版本、入口和 NSIS 构建契约测试。

### Task 2: 原生插件 ABI 与安全基础设施

**Files:**
- Create: `native/provisioning/CMakeLists.txt`
- Create: `native/provisioning/include/nsis_plugin.hpp`
- Create: `native/provisioning/src/plugin.cpp`
- Create: `native/provisioning/src/crypto.cpp`
- Create: `native/provisioning/src/json.cpp`
- Create: `native/provisioning/tests/provisioning_tests.cpp`
- Create: `scripts/build_provisioning_plugin.py`

**Interfaces:**
- Produces NSIS exports: `ProvisionDevice`, `StartProjectScan`, `PollProjectScan`, `PopulateProjectList`, `WriteConfiguration`, `VerifyCoreStatus`, `RevokeDevice`。
- Produces C++ helpers: `sha256_hex`, `json_escape`, `dpapi_protect`, `atomic_write`。

- [x] 写 C++ 失败测试，覆盖 Unicode JSON 转义、SHA-256、DPAPI round-trip、原子写入和导出符号清单。
- [x] 配置 CMake Win32 构建并确认测试在缺少实现时失败。
- [x] 实现最小 NSIS Unicode ABI、插件栈读取/写入和非敏感状态返回。
- [x] 实现 BCrypt SHA-256、CryptProtectData CurrentUser、敏感缓冲区清零和原子文件替换。
- [x] 构建 Win32 Release DLL，运行 `ctest --test-dir build/native-provisioning -C Release --output-on-failure`。

### Task 3: 原生项目扫描与可选项目状态

**Files:**
- Create: `native/provisioning/src/project_scan.cpp`
- Modify: `native/provisioning/src/plugin.cpp`
- Modify: `native/provisioning/tests/provisioning_tests.cpp`

**Interfaces:**
- `start_scan(root, max_depth=8, limit=100)` 异步扫描。
- `scan_snapshot() -> {state, visited, projects, truncated, error_code}`。
- `write_selected_projects(HWND list, path)` 写非敏感 UTF-8 JSON。

- [x] 写失败测试，覆盖 `.git` 目录/文件、`.svn`、嵌套项目、排除目录、Unicode、深度、100 项、junction 和不可访问目录。
- [x] 实现不跟随 reparse point 的后台扫描与线程安全快照。
- [x] 实现 ListView 填充和选择结果写入 staging 配置；扫描错误返回 warning，不返回 install failure。
- [x] 运行原生测试并对含 100 个项目的 fixture 做内存/耗时上限检查。

### Task 4: WinHTTP Bootstrap、DPAPI 与配置提交

**Files:**
- Create: `native/provisioning/src/http.cpp`
- Create: `native/provisioning/src/provision.cpp`
- Modify: `native/provisioning/src/plugin.cpp`
- Modify: `native/provisioning/tests/provisioning_tests.cpp`
- Modify: `server/internal/httpapi/router_test.go`

**Interfaces:**
- `provision(gateway, enrollment, user, install_root, projects_file) -> ProvisionResult`。
- `ProvisionResult` 只包含 `status_code, error_code, tenant_id, subject_id, device_id`。

- [x] 写 loopback fake Gateway 失败测试，覆盖 401、超时、5xx、身份不一致、成功 bootstrap/heartbeat 和 token 不出现在结果/日志。
- [x] 实现 WinHTTP TLS/HTTP 状态映射、JSON 请求响应和响应大小上限。
- [x] 在单次调用内生成 subject/device ID、bootstrap、DPAPI 加密 token、heartbeat、控制令牌和配置。
- [x] 实现 `.installing/<random>` staging、credential 回读、原子提交和失败清理。
- [x] 运行 C++、Go contract 和 secret-literal 测试。

### Task 5: NSIS MUI2 单窗口向导

**Files:**
- Create: `installer/windows/nsis/AetherisSetup.nsi`
- Create: `installer/windows/nsis/pages/Projects.nsh`
- Create: `installer/windows/nsis/pages/Enrollment.nsh`
- Create: `installer/windows/nsis/pages/Progress.nsh`
- Create: `installer/windows/nsis/assets/`
- Modify: `installer/windows/README_INSTALL.md`
- Test: `tests/test_nsis_script.py`

**Interfaces:**
- Build defines: `VERSION`, `CORE_EXE`, `PLUGIN_DLL`, `GATEWAY_URL`, `OUTPUT_DIR`。
- Installer output: `AetherisSetup-${VERSION}.exe`。

- [x] 写失败测试，解析 `.nsi/.nsh` 并断言 MUI2 目录页、可选项目页、enrollment 页、进度页、完成页、当前用户启动项和 `WriteUninstaller`。
- [x] 实现始终可见的 MUI2/nsDialogs 页面，不调用 `root.withdraw` 或任何 Python Setup。
- [x] 项目页实现异步扫描进度、项目多选和“稍后添加”；零项目允许 Next。
- [x] enrollment 页保留输入用于网络重试，并在成功后清空密码控件与 NSIS 变量。
- [x] Section 中安装 Core、调用 provisioning、写启动项/Uninstall、启动 Core并等待状态；失败停留在可重试页面。
- [x] 使用 `makensis` 构建并运行脚本契约测试。

### Task 6: ProjectRegistry 与 Core 热加载

**Files:**
- Create: `src/aetheris/project_registry.py`
- Modify: `src/aetheris/tray.py`
- Modify: `src/aetheris/status.py`
- Create: `tests/test_project_registry.py`
- Modify: `tests/test_tray.py`

**Interfaces:**
- `ProjectRegistry.load() -> ProjectSnapshot`。
- `ProjectRegistry.mutate(expected_revision, operation) -> ProjectSnapshot`。
- `TrayCore.reload_projects_if_changed() -> bool`。

- [x] 写失败测试，覆盖零项目、revision 冲突、原子添加/暂停/恢复/移除、非法路径和外部配置变化。
- [x] 实现 `project_revision`、文件锁、临时文件替换和审计记录。
- [x] 重构 TrayCore 按项目 diff 创建/释放 Git、SVN、Visual Studio adapter；进程采集不按项目复制。
- [x] 零项目时写 `waiting_for_project`，保持 heartbeat 和托盘运行。
- [x] 运行 ProjectRegistry、Tray、Core 状态和垂直链路测试。

### Task 7: 本地 Lens 动态项目界面与控制安全

**Files:**
- Modify: `src/aetheris/local_view.py`
- Modify: `src/aetheris/tray.py`
- Create: `tests/test_local_project_management.py`

**Interfaces:**
- `GET /api/projects`。
- `POST /api/projects/scan|add|pause|resume|remove`。
- 每个修改请求要求 `X-Aetheris-Control`、loopback Host 和匹配 Origin。

- [x] 写失败测试，覆盖非 loopback Host、错误 Origin、缺少 token、revision 冲突和全部项目操作。
- [x] 生成独立 DPAPI `control.credential`，页面启动时使用 HttpOnly loopback session，不把控制 token 放进 HTML/URL。
- [x] 实现本地 Lens 项目表格、扫描进度、目录浏览触发、授权、暂停、恢复和移除确认。
- [x] 托盘增加“管理项目”并指向本地 Lens 项目页。
- [x] 运行 Local Lens、ProjectRegistry 和浏览器安全测试。

### Task 8: 原生卸载、UI 自动化、发布与服务器验收

**Files:**
- Modify: `installer/windows/nsis/AetherisSetup.nsi`
- Create: `tests/windows/test_nsis_installer_ui.py`
- Modify: `admin-web/src/App.tsx`
- Modify: `deploy/.env.example`
- Modify: `docs/operations/implementation-log.md`
- Modify: `docs/operations/server-implementation-log.md`

**Interfaces:**
- `Uninstall.exe` 默认保留非敏感配置/data/logs，始终删除 credentials、程序和启动项。
- `/downloads/client` 返回 `AetherisSetup-0.4.1.exe`。

- [x] 写失败测试，覆盖卸载正常退出 Core、凭据撤销、默认保留数据和显式删除数据。
- [x] 实现 NSIS uninstall section，不调用脚本；服务端不可达记录 `revocation_pending`。
- [x] 用 loopback fake Gateway 执行 UI Automation：可见主窗口、目录选择、跳过项目、错误 enrollment、重试、完成页和 Setup 退出。
- [x] 验证进程树不存在 cmd、PowerShell、Python Setup，安装后只剩逻辑 Core。
- [x] 运行完整 Python、C++、Go、Admin Web、Model Gateway 测试和 secret 扫描。
- [x] 构建 Setup/Core checksum，本机完成安装/动态项目/卸载实测。
- [x] 上传到 `/opt/aetheris` staging，验证哈希后原子切换 Go 服务与 `/downloads/client`。
- [x] 真实验证错误 enrollment、bootstrap、heartbeat、ingest、duplicate、Admin 设备列表、HTTP 完整下载哈希和 100 GB 磁盘限制。
