# Aetheris Windows Setup 与 Core 0.4.0 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: 使用 `superpowers:executing-plans` 按任务逐项实现，步骤使用复选框跟踪。

**目标：** 交付用户自选安装目录的独立 Setup、只负责运行的独立 Core、多 Git/SVN 项目采集、可靠 enrollment/heartbeat、单实例与可见状态，并发布统一 0.4.0。

**架构：** Setup 负责项目扫描、bootstrap、DPAPI credential、配置和启动项；Core 只加载已安装配置，维护 heartbeat、采集、队列、托盘和本地状态。服务端 bootstrap/heartbeat 与 0.4.0 契约对齐。

**技术栈：** Python 3.11+ 标准库、Tkinter、ctypes Windows DPAPI/Mutex、PyInstaller、Go Server、PostgreSQL。

**设计文档：** `docs/superpowers/specs/2026-09-06-windows-setup-core-design.md`

## 全局约束

- 所有新增或修改文档使用中文。
- 版本唯一值为 `0.4.0`。
- 安装目录由用户选择，默认目录仅作为建议。
- 项目扫描最大深度 8、最多 100 项、不跟随 symlink/junction。
- 支持 `.git` 目录、`.git` 文件和 `.svn` 目录。
- device token 使用 DPAPI CurrentUser 加密，不写明文 token。
- 安装成功必须经过 bootstrap、credential round-trip、heartbeat、Core 启动状态验证。
- 注册错误与采集错误独立，不能互相覆盖。
- Setup/Core 发布包不包含 enrollment secret、device token 或服务器密码。

### Task 1：版本、项目扫描与 SVN adapter

**Files:**
- Modify: `src/aetheris/version.py`, `pyproject.toml`
- Create: `src/aetheris/projects.py`, `src/aetheris/adapters/svn.py`
- Test: `tests/test_project_scan.py`, `tests/test_svn_adapter.py`

**Interfaces:**
- `discover_projects(root, max_depth=8, limit=100) -> ScanResult`
- `ProjectSource(path: Path, vcs: Literal["git", "svn"])`
- `SvnAdapter.collect(root) -> list[dict]`

- [x] 写 Git 文件/目录、SVN、嵌套、跳过目录、深度与数量上限测试并确认失败。
- [x] 实现不跟随 reparse point 的递归扫描和只读 SVN XML adapter。
- [x] 运行 `python -m unittest tests.test_project_scan tests.test_svn_adapter -v`。

### Task 2：身份、DPAPI credential 与 Gateway bootstrap

**Files:**
- Create: `src/aetheris/identity.py`, `src/aetheris/credentials.py`
- Modify: `src/aetheris/gateway.py`
- Test: `tests/test_identity.py`, `tests/test_credentials.py`, `tests/test_gateway.py`

**Interfaces:**
- `subject_id(identifier)`, `device_id(identifier, machine_guid)`。
- `CredentialStore.write/read`，支持注入 protector 测试，Windows 默认 DPAPI。
- `GatewayClient.bootstrap_device(...)` 与 `heartbeat()`。

- [x] 写身份稳定性、credential round-trip/损坏和 bootstrap/heartbeat HTTP 测试并确认失败。
- [x] 实现接口；移除 0.4.0 send 前隐式 register。
- [x] 运行对应 Python 测试。

### Task 3：独立 Setup 状态机

**Files:**
- Create: `src/aetheris/setup_app.py`, `scripts/setup_entry.py`, `scripts/build_windows_setup.py`
- Test: `tests/test_setup_app.py`, `tests/test_setup_build.py`

**Interfaces:**
- `InstallRequest`、`Installer.install(request) -> InstallResult`。
- `Installer` 注入 gateway、credential store、startup registrar 和 launcher。
- GUI 选择安装目录、扫描根、项目多选、主体标识和 enrollment code。

- [x] 写错误 enrollment 不成功、用户目录布局、heartbeat 验证、稳定启动项和 legacy 安全删除测试并确认失败。
- [x] 实现原子复制、状态日志、配置、启动项、Core 启动轮询和中文 GUI。
- [x] 运行 Setup 测试和 compileall。

### Task 4：Core 多项目、状态、heartbeat、单实例与托盘

**Files:**
- Create: `src/aetheris/status.py`, `src/aetheris/single_instance.py`
- Modify: `src/aetheris/tray.py`, `src/aetheris/local_view.py`
- Test: `tests/test_core_status.py`, `tests/test_single_instance.py`, `tests/test_tray.py`, `tests/test_local_view.py`

**Interfaces:**
- `CoreStatus.update_registration/update_capture/update_queue`。
- `SingleInstance.acquire() -> bool`。
- `TrayConfig.project_roots` 和 `credential_file`。
- Core heartbeat 60 秒，失败退避 5 秒到 300 秒。

- [x] 写注册错误不被采集覆盖、多项目配置、单实例名、状态页字段和重试策略测试并确认失败。
- [x] 实现多项目 Git/SVN 循环、独立状态、mutex、托盘通知与首次状态页。
- [x] 运行 Core/Tray/Local View 测试。

### Task 5：Go bootstrap/heartbeat 契约

**Files:**
- Modify: `server/internal/devices/service.go`, `server/internal/httpapi/router.go`
- Modify: `contracts/openapi.yaml`
- Test: `server/internal/devices/service_test.go`, `server/internal/httpapi/router_test.go`

**Interfaces:**
- bootstrap 接收可选 `subject_name`。
- heartbeat 返回 tenant/subject/device/work_role/server_time。

- [x] 写 JSON contract 和 heartbeat response 测试并确认失败。
- [x] 实现服务端契约且保持旧 endpoint。
- [x] 运行 `go -C server test ./...` 和 `go -C server vet ./...`。

### Task 6：构建、发布与实机链路验证

**Files:**
- Modify: `scripts/build_windows_core.py`, `scripts/build_core_bundle.py`, `installer/windows/README_INSTALL.md`
- Modify: `admin-web/src/App.tsx`, `deploy/.env.example`
- Modify: `docs/operations/server-implementation-log.md`

**Interfaces:**
- 产物 `AetherisCore-0.4.0.exe`、`AetherisSetup-0.4.0.exe` 及 SHA-256。
- `/downloads/client` 默认返回 Setup。

- [x] 运行完整 Python/Go/Admin 测试。
- [x] 构建 Core，再构建内嵌 Core 的 Setup；扫描二进制确认无敏感字面量。
- [x] 上传 Setup/Core/checksum，更新服务端 `CLIENT_DOWNLOAD_FILE` 并重启 Go Server。
- [x] 验证下载文件、错误 enrollment、正确 bootstrap/heartbeat/ingest 和设备管理数据源。
- [x] 更新中文实施记录；保留 0.3.x 回滚文件。
