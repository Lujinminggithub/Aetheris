# Aetheris Windows Setup 与 Core 分离设计

**日期：** 2026-09-06  
**状态：** 待实现  
**版本目标：** 0.4.0  
**范围：** Windows Setup、Windows Core、多项目发现、Git/SVN 适配、设备 enrollment、托盘状态、服务端 bootstrap 兼容和发布。

## 1. 问题与目标

当前 `AetherisCore-0.3.x.exe` 同时承担首次配置、启动项注册、采集和托盘运行。它实际在下载目录中就地运行，没有稳定安装边界。安装成功只检查服务端 `/healthz`，不检查设备注册；客户端仍调用旧注册 endpoint 并把用户输入 token 直接当作 device token。注册失败随后会被采集循环的 `running` 状态覆盖。

0.4.0 将安装与运行彻底分离：

- `AetherisSetup-0.4.0.exe`：安装、项目发现、enrollment、配置、启动项和首次启动验证。
- `AetherisCore-0.4.0.exe`：托盘、状态页、heartbeat、采集、本地队列和上传，不包含安装向导。
- 安装目录由用户选择；默认目录只是建议值。
- 安装成功必须以 bootstrap、device token 持久化、heartbeat 和 Core 启动成功为准。

## 2. 组件边界

```text
AetherisSetup.exe
  选择安装目录
  -> 选择扫描根目录
  -> 递归发现 Git/SVN 项目
  -> 确认项目与主体标识
  -> enrollment bootstrap
  -> DPAPI 保存 device token
  -> 写配置/启动项
  -> 启动 AetherisCore.exe
  -> heartbeat + 本地状态验证
  -> 安装成功

AetherisCore.exe
  单实例锁
  -> 加载配置和 DPAPI device token
  -> heartbeat 重试
  -> 多项目 adapters
  -> 本地队列/上传
  -> 托盘 + 本地状态页
```

Setup 不包含采集循环。Core 不弹出安装目录、enrollment code 或项目选择向导。二者只通过版本化配置文件和 credential store 交接。

## 3. 安装目录与文件布局

Setup 提供目录选择器，默认建议 `%LOCALAPPDATA%\Aetheris`，但用户可以选择任意当前用户可写目录。Setup 不要求管理员权限。

安装布局：

```text
<用户选择目录>\
  AetherisCore.exe
  config\aetheris.json
  config\device.credential
  data\client.db
  data\core-status.json
  logs\core.log
  uninstall.ps1
```

启动项固定指向：

```text
"<用户选择目录>\AetherisCore.exe" --config "<用户选择目录>\config\aetheris.json"
```

Setup 只覆盖它拥有的命名文件，不删除未知文件或整个用户目录。升级时发现运行中的 Core，提示用户退出后再替换；不静默终止进程。

Setup 完成后可以保留在下载目录。安装后的 Core 和启动项不依赖 Setup 或原下载路径。

## 4. Setup 状态机

安装阶段固定为：

```text
select_install_dir
scan_projects
confirm_projects
collect_identity
bootstrap_device
store_credential
write_config
write_startup
launch_core
verify_heartbeat
complete
```

任何阶段失败都写入 `<安装目录>\logs\setup.log` 和 `install-status.json`，显示中文错误并停在可重试状态。只有全部阶段成功才显示“安装完成”。

回滚规则：

- bootstrap 前失败：删除本次创建的临时文件，不修改启动项。
- bootstrap 后、配置前失败：保留已签发 token 的加密 credential，允许重试，不再次生成设备身份。
- 启动项写入后验证失败：删除本次启动项，但保留安装文件和诊断日志。
- 不删除用户原有目录或其他应用文件。

## 5. 项目递归发现

用户选择一个扫描根目录。Setup 递归发现：

- Git working tree：目录中存在 `.git` 目录或 `.git` 文件。
- SVN working copy：目录中存在 `.svn` 目录。

扫描规则：

- 最大深度 8 层，扫描根目录为第 0 层。
- 最多返回 100 个项目；达到上限时明确提示结果已截断。
- 不跟随符号链接、junction 或 reparse point，避免循环和跨盘扫描。
- 识别当前目录 marker 后仍允许继续查找嵌套仓库，但不进入 `.git`、`.svn` 内部。
- 跳过 `node_modules`、`.venv`、`venv`、`dist`、`build`、`.cache`、`__pycache__`、`bin`、`obj`。
- 路径按 Windows 大小写不敏感规则去重，结果按路径排序。

发现结果在 Setup 中显示为多选列表，默认全选。至少选择一个项目才能继续。配置保存：

```json
{
  "scan_root": "E:\\code",
  "project_roots": [
    {"path": "E:\\code\\repo-a", "vcs": "git"},
    {"path": "E:\\code\\repo-b", "vcs": "svn"}
  ]
}
```

保留旧 `project_root` 只用于读取 0.3.x 配置；0.4.0 写入时以 `project_roots` 为准。

## 6. 主体与设备身份

Setup 显示“用户标识”，默认值为 `DOMAIN\USERNAME`，允许用户修改为组织内稳定标识。规范化规则为去除首尾空格并 `casefold`。

客户端派生：

```text
subject_id = "subject-" + SHA256(normalized_user_identifier)[0:16]
device_id  = "device-" + SHA256(machine_guid + normalized_user_identifier)[0:16]
```

同一用户在多台终端填写相同用户标识时得到相同 `subject_id`，设备 ID 保持不同。事件只携带不透明 ID。Bootstrap 可以附带 `subject_name` 用于 Admin 显示，但服务端以凭证绑定的 tenant/subject/device 为准。

## 7. Enrollment 与凭据

Setup 把用户输入称为 `enrollment code`，不能称为 device token。

请求：

```http
POST /api/v1/device/bootstrap
{
  "enrollment_secret": "...",
  "device_id": "device-...",
  "subject_id": "subject-...",
  "subject_name": "DOMAIN\\USERNAME",
  "client_version": "0.4.0",
  "hostname": "WORKSTATION-01"
}
```

响应中的 `device_token` 只返回一次。Setup 立即使用 Windows DPAPI CurrentUser 加密后写入 `config\device.credential`；不再写明文 `gateway.env`。文件 ACL 仅允许当前用户读取和写入。

随后调用：

```http
POST /api/v1/device/heartbeat
Authorization: Bearer <device_token>
```

Heartbeat 返回 200 且响应 `device_id/subject_id` 与配置一致，才允许进入下一阶段。

## 8. 旧配置升级

0.4.0 检测以下 legacy 状态：

- 存在 `gateway.env`，但不存在 `device.credential`。
- 配置缺少 `credential_kind=device_token`。
- 配置只有 `project_root`，没有 `project_roots`。

旧明文 token 不直接迁移为 device token。Setup 要求重新输入 enrollment code，完成 bootstrap 后写入 DPAPI credential。旧 `project_root` 作为扫描起点重新发现项目并让用户确认。

升级成功后删除旧 `gateway.env`；删除前确认新 credential 已解密并完成 heartbeat。失败时保留旧文件，避免不可恢复。

## 9. Core heartbeat 与重试

Core 启动后立即 heartbeat，成功后每 60 秒执行一次。失败策略：

- 网络错误、超时、HTTP 5xx：5 秒起始指数退避，最大 5 分钟，成功后恢复 60 秒周期。
- HTTP 401/403：状态标记 `credential_invalid`，保持每 5 分钟重试，并通过本地状态页提示重新 enrollment。
- HTTP 4xx 参数错误：状态标记 `client_configuration_error`，不高频重试。

事件上传不再隐式调用 `register_device`。Heartbeat 与 ingest 是独立链路，互不覆盖状态。

## 10. 状态模型与日志

`core-status.json` 分离保存：

```json
{
  "registration": {
    "state": "registered|pending|credential_invalid|error",
    "last_heartbeat_at": "RFC3339|null",
    "last_error": ""
  },
  "capture": {
    "state": "running|paused|error",
    "project_count": 8,
    "last_capture_at": "RFC3339|null",
    "last_error": ""
  },
  "queue": {
    "queued": 0,
    "retrying": 0,
    "rejected": 0
  },
  "version": "0.4.0"
}
```

采集成功不能清除注册错误；heartbeat 成功也不能清除 adapter 错误。`logs\core.log` 只记录状态、错误码、设备 ID 和计数，不记录 token、命令正文、事件 payload 或原始 OCR。

## 11. 多项目采集

Core 为每个 `project_roots` 条目建立独立 adapter：

- Git：复用 `GitAdapter`，采集 commit metadata 和 diff aggregate，不读取 patch body。
- SVN：新增 `SvnAdapter`，只调用只读 `svn info --xml` 和 `svn status --xml`；上传 working-copy revision、repository UUID hash、最后变更时间和状态数量，不上传 repository URL、密码或文件内容。
- 系统没有 `svn.exe` 时，该项目状态显示 `adapter_unavailable`，其他 Git/SVN 项目继续采集。

相同事件的 fingerprint 必须包含项目 ID，避免不同项目之间错误去重。进程事件只有在能明确关联项目时才产生，不允许为每个项目复制同一个进程事件。

## 12. 单实例与托盘可见性

Core 使用 Windows named mutex：

```text
Local\AetherisCore-<current-user-sid-hash>
```

第二次启动检测到 mutex 后显示“Core 已在运行”，打开本地状态页并退出。PyInstaller one-file bootloader 父进程和应用子进程仍是一个逻辑实例，不按进程数量判断重复。

托盘策略：

- `pystray.Icon.run()` 仍在应用主线程运行。
- icon ready 后发送 Windows 通知：“Aetheris Core 已启动，状态页可用于检查注册和采集”。
- 安装后的第一次 Core 启动自动打开一次本地状态页，随后把 `show_status_on_first_run` 原子更新为 `false`。
- 托盘菜单首项显示注册和采集摘要，提供“打开状态页”“立即 heartbeat”“立即采集”“退出”。
- Windows 决定图标位于主通知区域或折叠区域；应用不修改用户的通知区固定设置。

## 13. 本地状态页

本地 Lens Host 固定绑定 `127.0.0.1`，端口优先使用配置值，冲突时选择动态端口并写入状态文件。页面至少显示：

- 服务端地址和最近 heartbeat。
- 已注册/待注册/凭据失效状态。
- 已授权项目清单、VCS 类型和 adapter 状态。
- 最近采集时间、队列数量和最后错误。
- “立即 heartbeat”和“立即采集”操作。

本地 API 不接受非 loopback Host，不向服务端暴露。

## 14. 服务端调整

Go Server bootstrap request 增加可选 `subject_name`。创建新 subject 时作为 `display_name`；已存在 subject 不因其他终端 bootstrap 被静默改名。

Heartbeat 响应包含：

```json
{
  "status": "online",
  "tenant_id": "tenant-...",
  "subject_id": "subject-...",
  "device_id": "device-...",
  "work_role": null,
  "server_time": "RFC3339"
}
```

服务端继续保留 0.3.x 兼容 endpoint，但 0.4.0 Setup/Core 只能调用 `/api/v1/device/bootstrap`、`/api/v1/device/heartbeat` 和 `/api/v1/ingest`。

## 15. 构建与发布

版本唯一来源为 `src/aetheris/version.py`，值为 `0.4.0`。构建顺序：

1. 使用 PyInstaller `--onefile --windowed` 构建 `AetherisCore-0.4.0.exe`。
2. 使用独立入口构建 `AetherisSetup-0.4.0.exe`，把 Core 作为只读内嵌二进制。
3. 生成两个 EXE 的 SHA-256。
4. 服务端默认 `/downloads/client` 指向 Setup，不再指向 Core。
5. Admin Web 下载文案显示“下载 Windows 安装程序”。

发布目录保留上一个版本和 checksum 用于回滚，但默认下载只能指向一个经过验证的 Setup 文件。

## 16. 测试与验收

### 自动化测试

- 项目扫描：Git 目录、Git 文件、SVN、嵌套项目、深度限制、跳过目录、junction、100 项上限。
- 身份：相同用户跨设备 subject 一致，同设备 ID 稳定，不同机器 device 不同。
- DPAPI：加密/解密 round-trip、错误用户/损坏数据失败、legacy token 不误迁移。
- Bootstrap：成功保存签发 token；401/网络错误不能显示安装成功。
- Heartbeat：60 秒周期、5 秒到 5 分钟退避、401 状态持久化、成功恢复。
- 状态：注册错误不被采集成功覆盖。
- 单实例：第二实例打开状态页并退出。
- Git/SVN adapters：只读命令、无 patch/URL/凭据/文件正文。
- 构建：Setup 与 Core 两个 EXE 版本一致，包内不含 enrollment secret 或 device token。

### 实机验收

1. 用户选择非默认安装目录，安装成功后删除下载目录，Core 仍可启动。
2. 递归扫描根目录发现 Git 和 SVN working copy，并允许多选。
3. 错误 enrollment code 明确失败，不能创建启动项或显示成功。
4. 正确 enrollment 后 PostgreSQL 出现一个 subject/device，heartbeat 为 online。
5. Admin Web 能看到设备和主体。
6. Core 状态页显示注册、项目、采集和队列的独立状态。
7. 第二次启动不产生第二个应用实例。
8. Windows 通知区折叠区域可看到图标，首次启动可收到通知并打开状态页。
9. 服务端不可用后恢复，Core 自动重连且不丢失本地队列。
10. 服务端默认下载文件、Setup、Core 和上报版本全部为 0.4.0。

