# Aetheris

Aetheris is a privacy-preserving developer work signal pipeline. The first runnable slice targets Windows 10/11 Core capture and a private-server Gateway.

## Current vertical slice

The current implementation proves this path:

```text
Core process/project fixture
  -> consent and project-root authorization
  -> deterministic redaction
  -> bounded local SQLite queue
  -> authenticated Gateway
  -> idempotent server SQLite event store
```

Screenshots are not part of this slice and are never persisted. The server receives only project-attributed, redacted `AetherisEvent` envelopes.

The first read-only source adapters are also present: `GitAdapter` emits the latest commit metadata and aggregate working-tree `--numstat` counts, while `TerminalAdapter` tails PowerShell/Windows Terminal PSReadLine history. Neither adapter collects raw patch bodies, and author email is hashed before returning data.

Windows 构建会生成原生 NSIS `AetherisSetup-0.4.15.exe`，内嵌 `AetherisCore-0.4.15.exe` 和原生 `AetherisCoreService.exe`。Setup 提供项目扫描、设备 enrollment/身份复用、DPAPI 凭据和 Windows 服务安装；服务在活动控制台用户会话中监管托盘 Core，Core 负责 Git/SVN、终端、IDE、AI session、Visual Studio 和浏览器采集。

## Requirements

- Python 3.11 or newer
- Windows PowerShell for the launcher (the Python modules also run on other development hosts)

The test suite uses only the Python standard library; no network or external service is required.

## Run tests

```powershell
python -m unittest discover -s tests -v
python -m compileall src tests
```

## 运行本地服务端

```powershell
docker compose -f deploy/docker-compose.yml up -d postgres
$env:DATABASE_URL = "postgres://aetheris:local-development-only@127.0.0.1:54329/aetheris?sslmode=disable"
$env:DEVICE_ENROLLMENT_SECRET = "local-enrollment-secret"
$env:ADMIN_BOOTSTRAP_PASSWORD = "仅用于本地开发的长密码"
go -C server run ./cmd/aetheris-migrate
go -C server run ./cmd/aetheris-server
```

Go Server 暴露 `GET /healthz`、新版 `/api/v1/*`，并保留旧版 Core 兼容路径 `/v1/events`。生产凭据不得放入 PowerShell 历史、源代码或日志，必须使用外部 secret 机制。

Admin Web 是独立的 React/Vite 静态前端，由 Go Server 托管 `/admin/`；它通过 HttpOnly session cookie 调用 `/api/v1/admin/*`，不直接连接数据库。个人事件审阅仍属于客户端 Local Lens，本线程不修改其实现。

Admin Web 当前包含总览 KPI、最近活动、模型健康、设备与主体管理、工作角色分配、事件筛选/详情/归档和审计日志视图。

“个人效能”页面提供可解释的活跃时段、工作会话、专注时段、上下文切换、活动构成、项目/工作角色分布、覆盖率和证据下钻；不提供员工排名或黑盒综合总分。

## Download the Windows installer from the private server

- Install page: `http://192.168.78.138:8080/`
- Download catalog: `http://192.168.78.138:8080/downloads/`
- Windows installer: `http://192.168.78.138:8080/downloads/client`
- Versioned artifact: `AetherisSetup-0.4.15.exe`（把用户态 Core 安装到自选目录，把监管服务安装到 Program Files）

下载 Setup EXE 并双击运行。用户选择安装目录和 Git/SVN 扫描根目录，确认项目与用户标识后输入 enrollment code；只有 bootstrap、DPAPI credential、heartbeat 和 Core 启动全部验证成功，Setup 才显示安装完成。

## Run the safe Core demo

Create or select a project root containing a `.git` directory, start the Gateway, then run:

```powershell
$env:PYTHONPATH = "src"
python -m aetheris.cli demo --server-url http://127.0.0.1:8080 --token beta-local-token --project C:\path\to\authorized\repo --queue .\data\client.db
```

The demo uses a fixed harmless process fixture. Real adapters and Windows process discovery are added only after their consent, project authorization, and redaction tests exist.

## Documents

- Architecture: `docs/superpowers/specs/2026-09-06-aetheris-architecture-design.md`
- Implementation plan: `docs/superpowers/plans/2026-09-06-aetheris-vertical-slice-plan.md`
- Server preflight: `docs/operations/server-read-only-preflight.md`

## 服务端（新架构）

服务端已经按独立组件拆分：Go Server、PostgreSQL、React/Vite Admin Web，以及可选的 Python Model Gateway（Ollama 默认、Dify 可选）。服务端设计、部署和运维文档使用中文，详见：

- `docs/superpowers/specs/2026-09-06-aetheris-server-architecture-design.md`
- `docs/superpowers/plans/2026-09-06-aetheris-server-implementation-plan.md`
- `docs/operations/server-deployment.md`
- `docs/operations/server-operations.md`

本地服务端启动需要 PostgreSQL：

```powershell
docker compose -f deploy/docker-compose.yml up -d postgres
$env:DATABASE_URL = "postgres://aetheris:local-development-only@127.0.0.1:54329/aetheris?sslmode=disable"
$env:DEVICE_ENROLLMENT_SECRET = "local-enrollment-secret"
$env:ADMIN_BOOTSTRAP_PASSWORD = "仅用于本地开发的长密码"
go -C server run ./cmd/aetheris-migrate
go -C server run ./cmd/aetheris-server
```
