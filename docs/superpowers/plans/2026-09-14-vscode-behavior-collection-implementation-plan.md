# Aetheris VS Code 行为采集实施计划

> **供执行代理使用：** 必须使用 `superpowers:subagent-driven-development`（推荐）或 `superpowers:executing-plans`，按任务逐项实施。所有步骤使用复选框跟踪。

**目标：** 交付默认安装但允许取消和停用的 Aetheris VS Code 扩展，可靠记录授权项目内的文件打开、编辑、保存、关闭、工作区变化和扩展状态，并贯通 Core、Gateway、Forge、Work Episode、Lens 和 Admin Web。

**架构：** VS Code 扩展只产生不含代码正文的行为元数据，通过当前用户专属 Windows 命名管道发送给 Core；Core 完成协议校验、项目授权、路径相对化、脱敏、SQLite 入队和上传。Go Server 保存组件状态和原始证据，Forge、Work Episode、Pulse、Nexus 消费规范事实；Windows 原生窗口适配器只作为运行状态兜底。

**技术栈：** TypeScript 5.7、VS Code Extension API 1.96、Node.js、Vitest、`@vscode/test-electron`、Python 3.12、Windows Named Pipe/ctypes、SQLite、Go 1.22、PostgreSQL 14、React 19、NSIS 3、原生 C++ Provisioning DLL。

**规格：** `docs/superpowers/specs/2026-09-14-vscode-behavior-collection-design.md`

## 全局约束

- 首期仅支持 Windows 10/11、VS Code Stable 和本机文件工作区。
- 扩展 ID 固定为 `aetheris.aetheris-vscode`，桥接协议版本固定为 `1`。
- 命名管道固定为 `\\.\pipe\Aetheris.VSCode.Bridge.v1`，仅允许当前用户 SID 和 SYSTEM。
- 单帧最大 64KB，单批最多 100 条；扩展缓存最大 8MB、最长 24 小时。
- Core SQLite 总缓存继续执行 512MB/7 天边界。
- 只处理 `file` URI 且文件必须位于授权项目根目录；远程宿主统一为 `unsupported_remote_host`。
- 禁止保存或上传代码正文、完整 diff、剪贴板、按键、终端输出、第三方扩展配置和私有存储。
- `contentChanges.text` 只允许读取长度；实际字符串不得进入对象、日志、缓存、异常或测试快照。
- 绝对路径只用于设备本地授权匹配，Core 入队前必须删除。
- 安装、升级和卸载直接启动 `Code.exe`，不得使用 CMD 或 PowerShell。
- 用户取消安装、停用或卸载后不得自动恢复扩展，也不得补采停用期间行为。
- 当前工作区没有 `.git`。任务末尾先执行 `git rev-parse --is-inside-work-tree`；若仍不是仓库，不运行提交命令，在 `docs/operations/implementation-log.md` 记录任务、测试和产物哈希。

## 文件结构

### 新增 VS Code 扩展

- `vscode-extension/package.json`：扩展清单、命令、版本和构建入口。
- `vscode-extension/tsconfig.json`：严格 TypeScript 配置。
- `vscode-extension/src/protocol.ts`：桥接协议和行为事件类型。
- `vscode-extension/src/privacy.ts`：URI、路径和文本字段约束。
- `vscode-extension/src/edit_aggregator.ts`：编辑长度聚合，不持有文本。
- `vscode-extension/src/collector.ts`：打开、保存、关闭、工作区事件采集。
- `vscode-extension/src/extension_inventory.ts`：扩展集合差异。
- `vscode-extension/src/bridge.ts`：命名管道连接、握手、确认和重试。
- `vscode-extension/src/spool.ts`：8MB/24 小时有界队列。
- `vscode-extension/src/extension.ts`：激活、停用和生命周期编排。
- `vscode-extension/test/*.test.ts`：纯单元测试。
- `vscode-extension/test/integration/*.test.ts`：真实 VS Code Electron 集成测试。

### Core

- `src/aetheris/vscode_protocol.py`：帧解析、字段验证和状态模型。
- `src/aetheris/vscode_bridge.py`：CurrentUser 命名管道服务器、ACK 和会话健康。
- `src/aetheris/vscode_events.py`：路径授权、相对化和 AetherisEvent 投影。
- `src/aetheris/vscode_installation.py`：VS Code/VSIX 检测和直接进程安装控制。
- `src/aetheris/tray.py`：桥接生命周期、队列和健康快照接入。
- `src/aetheris/local_view.py`：本地 Lens 状态和启停接口。
- `tests/test_vscode_*.py`：协议、桥接、隐私、安装和本地 UI 测试。

### 服务端与前端

- `server/migrations/016_vscode_component_state.sql`：组件状态字段和索引。
- `server/internal/adapterhealth/{model.go,repository.go}`：`component_state` 持久化。
- `server/internal/cleaning/{rules.go,rules_test.go}`：IDE 行为规范事实。
- `server/internal/episodes/{builder.go,model_test.go}`：工作片段动作与验证。
- `server/internal/retrieval/{document.go,document_test.go}`：安全摘要索引。
- `admin-web/src/pages/VSCodeIntegrationPage.tsx`：管理员状态页。
- `admin-web/src/pages/VSCodeIntegrationPage.test.tsx`：状态展示测试。
- `admin-web/src/api/{types.ts,client.ts}`、`admin-web/src/App.tsx`：API 和导航。

### 安装与构建

- `scripts/build_vscode_extension.py`：确定性构建 VSIX、校验内容和生成 SHA-256。
- `scripts/build_windows_setup.py`：把 VSIX 作为必需构建输入传给 NSIS。
- `installer/windows/nsis/AetherisSetup.nsi`：默认选中安装项、升级和卸载。
- `native/provisioning/include/aetheris/vscode_extension.hpp`：原生安装接口。
- `native/provisioning/src/vscode_extension.cpp`：直接启动 `Code.exe`。
- `native/provisioning/src/{plugin.cpp,exports.def}`：NSIS 导出。
- `native/provisioning/tests/provisioning_tests.cpp`：命令行、退出码和回滚测试。

---

### 任务 1：定义桥接协议和事件契约

**状态：已完成（检查点 1）**

**文件：**
- 新建：`contracts/vscode-bridge-v1.schema.json`
- 新建：`vscode-extension/src/protocol.ts`
- 新建：`src/aetheris/vscode_protocol.py`
- 新建：`tests/test_vscode_protocol.py`
- 修改：`contracts/aetheris-event-v1.schema.json`

**接口：**
- 产生：TypeScript `BridgeEnvelope`、`BehaviorEvent`、`ComponentHeartbeat`。
- 产生：Python `parse_bridge_frame(raw: bytes) -> BridgeEnvelope`。
- 约定事件：`ide.file_opened`、`ide.file_edited`、`ide.file_saved`、`ide.file_closed`、`ide.workspace_changed`、`ide.extension_changed`。

- [ ] **步骤 1：编写协议失败测试**

```python
def test_rejects_source_text_and_oversized_frames():
    with self.assertRaises(BridgeProtocolError):
        parse_bridge_frame(json.dumps({
            "version": 1, "type": "events",
            "events": [{"event_type": "ide.file_edited", "source_text": "secret"}],
        }).encode())
    with self.assertRaises(BridgeProtocolError):
        parse_bridge_frame(b"x" * (64 * 1024 + 1))
```

- [ ] **步骤 2：运行红灯**

运行：`python -m unittest tests.test_vscode_protocol -v`  
预期：因 `aetheris.vscode_protocol` 不存在而失败。

- [ ] **步骤 3：实现严格协议**

```python
@dataclass(frozen=True)
class BridgeEnvelope:
    message_type: str
    session_id: str
    events: tuple[dict, ...]

def parse_bridge_frame(raw: bytes) -> BridgeEnvelope:
    if not 0 < len(raw) <= 64 * 1024:
        raise BridgeProtocolError("frame_size_invalid")
    value = json.loads(raw)
    reject_forbidden_keys(value, {"source_text", "text", "diff", "clipboard", "terminal_output"})
    validate_version_and_batch(value, version=1, max_events=100)
    return BridgeEnvelope(
        message_type=str(value["type"]),
        session_id=str(value["session_id"]),
        events=tuple(value.get("events", [])),
    )
```

- [ ] **步骤 4：验证契约**

运行：

```powershell
python -m unittest tests.test_vscode_protocol -v
python -m json.tool contracts\vscode-bridge-v1.schema.json > $null
```

预期：全部通过。

- [ ] **步骤 5：记录检查点**

若 Git 可用：

```powershell
git add contracts vscode-extension/src/protocol.ts src/aetheris/vscode_protocol.py tests/test_vscode_protocol.py
git commit -m "feat: define vscode bridge protocol"
```

否则在实施日志记录协议版本、事件类型和测试输出。

### 任务 2：搭建扩展并采集文件打开和工作区变化

**状态：已完成（检查点 1）**

**文件：**
- 新建：`vscode-extension/package.json`
- 新建：`vscode-extension/tsconfig.json`
- 新建：`vscode-extension/src/privacy.ts`
- 新建：`vscode-extension/src/collector.ts`
- 新建：`vscode-extension/src/extension.ts`
- 新建：`vscode-extension/test/collector.test.ts`

**接口：**
- 消费：任务 1 的 `BehaviorEvent`。
- 产生：`createCollector(vscodeApi, sink, clock): Collector`。
- 产生：`toAuthorizedFileMetadata(document, workspaceFolders): FileMetadata | null`。

- [ ] **步骤 1：编写打开文件隐私测试**

```typescript
it('只为活动编辑器中的本机文件生成打开事件', () => {
  const event = collector.activeEditorChanged(fileEditor('D:\\repo\\src\\app.ts', 'typescript'))
  expect(event).toMatchObject({
    event_type: 'ide.file_opened',
    workspace_path: 'D:\\repo',
    file_path: 'D:\\repo\\src\\app.ts',
    relative_path: 'src/app.ts',
    file_name: 'app.ts',
    extension: '.ts',
    language_id: 'typescript'
  })
  expect(JSON.stringify(event)).not.toContain('source_text')
  expect(collector.activeEditorChanged(outputEditor())).toBeNull()
})
```

- [ ] **步骤 2：运行红灯**

运行：`npm --prefix vscode-extension test -- --run collector.test.ts`  
预期：扩展包或 `createCollector` 不存在。

- [ ] **步骤 3：实现最小采集器**

```typescript
export function toAuthorizedFileMetadata(document: TextDocument, roots: Uri[]): FileMetadata | null {
  if (document.uri.scheme !== 'file') return null
  const match = roots.find(root => isDescendant(root.fsPath, document.uri.fsPath))
  if (!match) return null
  return {
    workspace_path: match.fsPath,
    file_path: document.uri.fsPath,
    relative_path: normalizeRelative(match.fsPath, document.uri.fsPath),
    file_name: path.basename(document.uri.fsPath),
    extension: path.extname(document.uri.fsPath).toLowerCase(),
    language_id: document.languageId,
  }
}
```

扩展到 Core 的本地对象可含 `workspace_path` 和 `file_path`，测试必须证明扩展持久化层和网络层没有这些字段。

- [ ] **步骤 4：验证扩展基础行为**

运行：

```powershell
npm --prefix vscode-extension test -- --run
npm --prefix vscode-extension run typecheck
```

预期：全部通过，无 TypeScript 错误。

- [ ] **步骤 5：记录检查点**

Git 可用时提交：`feat: collect vscode file open events`；否则记录测试和新增文件。

### 任务 3：实现编辑聚合、保存和关闭事件

**状态：已完成（检查点 1）**

**文件：**
- 新建：`vscode-extension/src/edit_aggregator.ts`
- 新建：`vscode-extension/test/edit_aggregator.test.ts`
- 修改：`vscode-extension/src/collector.ts`
- 修改：`vscode-extension/src/extension.ts`

**接口：**
- 产生：`EditAggregator.record(documentId, changes, at): void`。
- 产生：`EditAggregator.flush(documentId, reason, at): BehaviorEvent | null`。
- 产生：`EditAggregator.clearAll(): void`。

- [ ] **步骤 1：编写不保存文本的失败测试**

```typescript
it('只累计插入和删除长度', () => {
  aggregator.record('doc-1', [{ text: 'password=secret', rangeLength: 3 }], now)
  const event = aggregator.flush('doc-1', 'save', later)!
  expect(event.inserted_chars).toBe(15)
  expect(event.deleted_chars).toBe(3)
  expect(JSON.stringify(event)).not.toContain('password=secret')
})
```

- [ ] **步骤 2：运行红灯**

运行：`npm --prefix vscode-extension test -- --run edit_aggregator.test.ts`  
预期：因 `EditAggregator` 不存在而失败。

- [ ] **步骤 3：实现聚合器和刷新条件**

```typescript
record(id: string, changes: readonly TextDocumentContentChangeEvent[], at: number) {
  const aggregate = this.byDocument.get(id) ?? newAggregate(at)
  for (const change of changes) {
    aggregate.change_count += 1
    aggregate.inserted_chars += change.text.length
    aggregate.deleted_chars += change.rangeLength
  }
  this.byDocument.set(id, aggregate)
}
```

实现 30 秒空闲、200 次变更、保存、编辑器切换和关闭刷新；实际 `change.text` 不赋给任何字段。

- [ ] **步骤 4：验证编辑生命周期**

运行：`npm --prefix vscode-extension test -- --run`  
预期：打开、编辑、保存、关闭和停用清理测试全部通过。

- [ ] **步骤 5：记录检查点**

Git 可用时提交：`feat: aggregate vscode edit metadata`；否则更新实施日志。

### 任务 4：实现扩展集合变化和状态心跳

**状态：已完成（检查点 2）**

**文件：**
- 新建：`vscode-extension/src/extension_inventory.ts`
- 新建：`vscode-extension/test/extension_inventory.test.ts`
- 修改：`vscode-extension/src/extension.ts`
- 修改：`vscode-extension/src/protocol.ts`

**接口：**
- 产生：`snapshotExtensions(all): ExtensionSnapshot[]`。
- 产生：`diffExtensions(previous, current): BehaviorEvent[]`。
- 产生：每 30 秒一个 `ComponentHeartbeat`。

- [ ] **步骤 1：编写扩展差异失败测试**

```typescript
it('只记录扩展标识版本和激活状态', () => {
  const changes = diffExtensions(
    [{ id: 'vendor.tool', version: '1.0.0', is_active: false }],
    [{ id: 'vendor.tool', version: '1.1.0', is_active: true }]
  )
  expect(changes[0]).toMatchObject({ change: 'updated', extension_id: 'vendor.tool', version: '1.1.0' })
  expect(JSON.stringify(changes)).not.toContain('configuration')
})
```

- [ ] **步骤 2：运行红灯**

运行：`npm --prefix vscode-extension test -- --run extension_inventory.test.ts`  
预期：差异函数不存在。

- [ ] **步骤 3：实现快照差异和心跳**

心跳固定字段：

```typescript
{
  type: 'heartbeat', protocol_version: 1,
  extension_id: 'aetheris.aetheris-vscode', extension_version,
  vscode_version: vscode.version, host_kind: remoteName ? 'remote' : 'local',
  pending_events, sent_events, dropped_events
}
```

- [ ] **步骤 4：验证状态与隐私**

运行：`npm --prefix vscode-extension test -- --run`  
预期：扩展清单、激活变化和心跳测试通过。

- [ ] **步骤 5：记录检查点**

Git 可用时提交：`feat: report vscode extension state`；否则更新实施日志。

### 任务 5：实现命名管道客户端和有界扩展缓存

**状态：已完成（检查点 2）**

**文件：**
- 新建：`vscode-extension/src/bridge.ts`
- 新建：`vscode-extension/src/spool.ts`
- 新建：`vscode-extension/test/bridge.test.ts`
- 新建：`vscode-extension/test/spool.test.ts`
- 修改：`vscode-extension/src/extension.ts`

**接口：**
- 产生：`BridgeClient.connect(): Promise<void>`、`sendBatch(events): Promise<Ack>`。
- 产生：`Spool.enqueue(event)`、`claim(limit)`、`ack(ids)`、`clear()`。

- [ ] **步骤 1：编写断线和淘汰失败测试**

```typescript
it('Core 离线时保留元数据并按时间和大小淘汰', async () => {
  const now = Date.parse('2026-09-14T08:00:00Z')
  await spool.enqueue({ event_id: 'old', event_type: 'ide.file_opened', occurred_at: '2026-09-13T06:00:00Z' })
  await spool.enqueue({ event_id: 'new', event_type: 'ide.file_opened', occurred_at: '2026-09-14T07:00:00Z' })
  await spool.compact({ now, maxBytes: 8 * 1024 * 1024, maxAgeMs: 24 * 60 * 60 * 1000 })
  expect(await spool.ids()).toEqual(['new'])
  expect(await readSpoolText()).not.toContain('source_text')
})
```

- [ ] **步骤 2：运行红灯**

运行：`npm --prefix vscode-extension test -- --run bridge.test.ts spool.test.ts`  
预期：客户端和缓存类不存在。

- [ ] **步骤 3：实现长度前缀协议与 ACK**

```typescript
const socket = net.connect('\\\\.\\pipe\\Aetheris.VSCode.Bridge.v1')
socket.write(Buffer.concat([uint32le(payload.length), payload]))
```

ACK 必须包含 `accepted_ids` 和 `rejected[{event_id,reason_code}]`；只有 accepted ID 从缓存删除。

- [ ] **步骤 4：验证重连、幂等和边界**

运行：`npm --prefix vscode-extension test -- --run`  
预期：断线、部分 ACK、重连、8MB 和 24 小时测试通过。

- [ ] **步骤 5：记录检查点**

Git 可用时提交：`feat: add vscode bridge spool`；否则更新实施日志。

### 任务 6：实现 Core 命名管道接收和项目授权

**状态：已完成（检查点 3）**

**文件：**
- 新建：`src/aetheris/vscode_bridge.py`
- 新建：`src/aetheris/vscode_events.py`
- 新建：`tests/test_vscode_bridge.py`
- 新建：`tests/test_vscode_events.py`
- 修改：`src/aetheris/tray.py`

**接口：**
- 消费：任务 1 的 `parse_bridge_frame`。
- 产生：`VSCodeBridgeServer.start()`、`stop()`、`snapshot()`。
- 产生：`project_vscode_event(event, config) -> AetherisEvent`。

- [ ] **步骤 1：编写路径逃逸和正文拒绝失败测试**

```python
def test_projects_authorized_relative_path_and_removes_absolute_paths():
    event = project_vscode_event(open_event(r"D:\repo\src\app.py"), config_for(r"D:\repo"))
    assert event.payload["relative_path"] == "src/app.py"
    assert "D:\\repo" not in json.dumps(event.to_dict())

def test_rejects_path_outside_authorized_roots():
    with self.assertRaisesRegex(VSCodeEventRejected, "path_not_authorized"):
        project_vscode_event(open_event(r"D:\private\secret.py"), config_for(r"D:\repo"))
```

- [ ] **步骤 2：运行红灯**

运行：`python -m unittest tests.test_vscode_bridge tests.test_vscode_events -v`  
预期：模块不存在。

- [ ] **步骤 3：实现 CurrentUser 管道和投影**

```python
class VSCodeBridgeServer:
    def __init__(self, user_sid, on_events, max_frame=64 * 1024, max_batch=100):
        self.user_sid = user_sid
        self.on_events = on_events
        self.max_frame = max_frame
        self.max_batch = max_batch
        self._stop = threading.Event()

def project_vscode_event(value, config):
    root = resolve_authorized_root(value.file_path, config.authorized_roots)
    relative = Path(value.file_path).resolve().relative_to(root).as_posix()
    payload = value.safe_payload(relative_path=relative)
    project = ProjectResolver().resolve(root, config.authorized_roots)
    return AetherisEvent.create(
        value.event_type, tenant_id=config.tenant_id, subject_id=config.subject_id,
        device_id=config.device_id, project_id=project.project_id,
        session_id=value.session_id, source="core.vscode.extension",
        source_version=__version__, payload=payload,
        redaction_report={"rules": [], "replacement_count": 0},
        processing_grants=["server_ingest"], event_id=value.event_id,
    )
```

ACK 只在 `LocalQueue.enqueue()` 成功后返回 accepted。

- [ ] **步骤 4：验证 Core 边界**

运行：

```powershell
python -m unittest tests.test_vscode_protocol tests.test_vscode_bridge tests.test_vscode_events -v
python -m unittest tests.test_redaction_and_queue tests.test_schema_and_queue_stats -v
```

预期：全部通过。

- [ ] **步骤 5：记录检查点**

Git 可用时提交：`feat: receive vscode behavior events`；否则更新实施日志。

### 任务 7：实现组件启停状态和服务端健康链路

**状态：已完成（检查点 3）**

**文件：**
- 新建：`server/migrations/016_vscode_component_state.sql`
- 修改：`server/internal/adapterhealth/model.go`
- 修改：`server/internal/adapterhealth/repository.go`
- 修改：`server/internal/adapterhealth/repository_test.go`
- 修改：`server/internal/httpapi/adapter_health_handlers_test.go`
- 修改：`src/aetheris/adapter_health.py`
- 新建：`tests/test_vscode_component_state.py`

**接口：**
- 产生：健康字段 `component_state`、`component_version`、`protocol_version`、`last_component_heartbeat_at`、`pending_events`。
- 保持：现有 `state` 枚举不变。

- [ ] **步骤 1：编写迁移和投影失败测试**

```go
func TestVSCodeComponentStateRoundTrips(t *testing.T) {
    snapshot := Snapshot{AdapterID: "vscode_extension", State: "disabled", ComponentState: "paused_by_user", ComponentVersion: "0.1.0"}
    // 保存后读取，必须保持 component_state，且 SafeSnapshot 仅移除 TenantID。
}
```

- [ ] **步骤 2：运行红灯**

运行：`go -C server test ./internal/adapterhealth ./internal/httpapi -run VSCode -v`  
预期：`ComponentState` 字段不存在。

- [ ] **步骤 3：实现迁移和兼容映射**

```sql
ALTER TABLE adapter_health_snapshots
  ADD COLUMN IF NOT EXISTS component_state TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS component_version TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS protocol_version INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS last_component_heartbeat_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS pending_events INTEGER NOT NULL DEFAULT 0 CHECK (pending_events >= 0);
```

映射：用户取消/停用使用 health `disabled`；未安装使用 `source_missing`；等待激活使用 `idle`；失联、不兼容和协议错误使用 `error`。

- [ ] **步骤 4：验证服务端状态链路**

运行：

```powershell
go -C server test ./internal/adapterhealth ./internal/httpapi -v
python -m unittest tests.test_adapter_health tests.test_adapter_health_upload tests.test_vscode_component_state -v
```

预期：全部通过。

- [ ] **步骤 5：记录检查点**

Git 可用时提交：`feat: persist vscode component state`；否则更新实施日志。

### 任务 8：实现扩展安装状态检测和本地 Lens 控制

**状态：已完成（检查点 3）**

**文件：**
- 新建：`src/aetheris/vscode_installation.py`
- 新建：`tests/test_vscode_installation.py`
- 修改：`src/aetheris/tray.py`
- 修改：`src/aetheris/local_view.py`
- 修改：`tests/test_local_project_management.py`

**接口：**
- 产生：`VSCodeInstallation.inspect() -> InstallationState`。
- 产生：`install(vsix) -> OperationResult`、`uninstall() -> OperationResult`。
- 产生本地 API：`GET /api/vscode/status`、`POST /api/vscode/install|enable|pause|uninstall|clear-cache`。
- 产生本地 API：`GET /api/vscode/events?limit=20`，只返回已相对化和脱敏的行为元数据。

- [ ] **步骤 1：编写无 shell 和用户状态失败测试**

```python
def test_installer_launches_code_directly_without_shell():
    result = installation.install(Path("Aetheris.vsix"))
    assert runner.argv == [code_exe, "--install-extension", vsix, "--force"]
    assert runner.shell is False
    assert runner.creationflags & subprocess.CREATE_NO_WINDOW
```

- [ ] **步骤 2：运行红灯**

运行：`python -m unittest tests.test_vscode_installation tests.test_local_project_management -v`  
预期：安装控制器和 API 不存在。

- [ ] **步骤 3：实现状态机和 Lens 操作**

状态转移必须显式：

```text
install_declined -> install -> awaiting_activation
awaiting_activation -> heartbeat -> active
active -> pause -> paused_by_user
paused_by_user -> enable -> awaiting_activation
active|paused_by_user -> uninstall -> not_installed
```

Lens 操作继续要求回环 Host、控制 Cookie、Origin 和 `X-Aetheris-Control`。

Lens 页面显示最近 20 条本机 VS Code 行为，字段限定为事件类型、项目名称、相对路径、语言和时间；接口投影删除字符数量、绝对路径、扩展完整清单和内部错误详情。

- [ ] **步骤 4：验证 UI 和进程树约束**

运行：`python -m unittest tests.test_vscode_installation tests.test_local_project_management tests.test_hidden_process -v`  
预期：状态转移、CSRF 和无 CMD/PowerShell 测试通过。

- [ ] **步骤 5：记录检查点**

Git 可用时提交：`feat: manage vscode integration locally`；否则更新实施日志。

### 任务 9：接入 Forge、Work Episode、Pulse 和 Nexus

**状态：已完成（检查点 4）**

**文件：**
- 修改：`server/internal/cleaning/rules.go`
- 修改：`server/internal/cleaning/rules_test.go`
- 修改：`server/internal/activities/classify.go`
- 修改：`server/internal/episodes/builder.go`
- 修改：`server/internal/episodes/model_test.go`
- 修改：`server/internal/retrieval/document.go`
- 修改：`server/internal/retrieval/document_test.go`
- 修改：`server/internal/effectiveness/aggregate.go`
- 修改：`server/internal/effectiveness/aggregate_test.go`
- 修改：`admin-web/src/pages/EffectivenessPage.tsx`
- 修改：`admin-web/src/pages/EffectivenessPage.test.tsx`
- 修改：`admin-web/src/api/types.ts`
- 修改：`src/aetheris/local_episodes.py`
- 修改：`tests/test_local_episodes.py`

**接口：**
- 产生规范动作：打开代码文件、编辑代码文件、保存代码文件、切换工作区、开发扩展环境变化。
- Nexus 文档只包含项目、语言、事件类型和安全摘要。
- Pulse 产生 `ide_file_opened_events`、`ide_edit_sessions`、`ide_file_saved_events` 三个解释性计数，不产生字符数或人员排名。

- [ ] **步骤 1：编写规范化失败测试**

```go
func TestVSCodeEditFactNeverContainsPathOrCharacterCountsInRetrievalText(t *testing.T) {
    fact := SourceFact{
        FactID: "fact-vscode-1", TenantID: "tenant-1", RuleVersion: 2,
        EventType: "ide.file_edited", ActivityType: "ide", ActorOrigin: "human",
        QualityState: "accepted", Payload: map[string]any{
            "relative_path": "src/app.py", "language_id": "typescript",
            "inserted_chars": 120, "deleted_chars": 30,
        },
    }
    document, ok := BuildDocument(fact, "embeddinggemma")
    if !ok || strings.Contains(document.Content, "src/app.py") || strings.Contains(document.Content, "120") {
        t.Fatalf("unsafe retrieval document: %#v", document)
    }
    if !strings.Contains(document.Content, "编辑 TypeScript 文件") { t.Fatal(document.Content) }
}
```

- [ ] **步骤 2：运行红灯**

运行：`go -C server test ./internal/cleaning ./internal/activities ./internal/episodes ./internal/retrieval -run VSCode -v`  
预期：新事件尚未分类或生成安全摘要。

- [ ] **步骤 3：实现确定性投影**

规则：

```text
ide.file_opened    -> activity / ide / human / “打开代码文件”
ide.file_edited    -> activity / ide / human / “编辑代码文件”
ide.file_saved     -> activity / delivery / human / “保存代码文件”
ide.workspace_changed -> activity / ide / human / “切换工作区”
ide.extension_changed -> activity / ide / system / “开发扩展环境变化”
```

字符数量只保留在规范事实结构字段，不进入 Pulse 排名或 Nexus 文本。

Pulse 聚合按事件计数：`ide.file_opened` 增加打开数，`ide.file_edited` 增加编辑会话数，`ide.file_saved` 增加保存数。Admin Web 只展示当前主体/设备范围的计数和趋势，不提供员工比较视图。

- [ ] **步骤 4：验证下游链路**

运行：

```powershell
go -C server test ./internal/cleaning ./internal/activities ./internal/episodes ./internal/retrieval ./internal/effectiveness -v
python -m unittest tests.test_local_episodes tests.test_forge tests.test_p5_outputs -v
npm --prefix admin-web test -- --run src/pages/EffectivenessPage.test.tsx
```

预期：全部通过。

- [ ] **步骤 5：记录检查点**

Git 可用时提交：`feat: organize vscode behavior facts`；否则更新实施日志。

### 任务 10：实现 Admin Web VS Code 状态页面

**状态：已完成（检查点 4）**

**文件：**
- 新建：`admin-web/src/pages/VSCodeIntegrationPage.tsx`
- 新建：`admin-web/src/pages/VSCodeIntegrationPage.test.tsx`
- 修改：`admin-web/src/api/types.ts`
- 修改：`admin-web/src/api/client.ts`
- 修改：`admin-web/src/App.tsx`
- 修改：`admin-web/src/styles.css`

**接口：**
- 消费：`GET /api/v1/admin/adapter-health?adapter_id=vscode_extension`。
- 展示：组件状态、版本、协议、最近心跳、最近事件、缓存数量和固定原因码。

- [ ] **步骤 1：编写状态文案失败测试**

```tsx
it('区分用户停用和组件故障', async () => {
  vi.spyOn(api, 'listAdapterHealth').mockResolvedValue({ items: [
    { device_id: 'd1', adapter_id: 'vscode_extension', component_state: 'paused_by_user', component_version: '0.1.0', protocol_version: 1, state: 'disabled', capability_version: '1', discovered: 0, parsed: 0, skipped: 0, failed: 0, lag_seconds: 0, pending_events: 0 },
    { device_id: 'd2', adapter_id: 'vscode_extension', component_state: 'bridge_offline', component_version: '0.1.0', protocol_version: 1, state: 'error', capability_version: '1', discovered: 0, parsed: 0, skipped: 0, failed: 1, lag_seconds: 180, pending_events: 3 },
  ], count: 2 })
  render(<VSCodeIntegrationPage />)
  expect(await screen.findByText('用户已停用')).toBeInTheDocument()
  expect(screen.getByText('桥接连接中断')).toHaveClass('status-down')
})
```

- [ ] **步骤 2：运行红灯**

运行：`npm --prefix admin-web test -- --run src/pages/VSCodeIntegrationPage.test.tsx`  
预期：页面不存在。

- [ ] **步骤 3：实现安静的管理状态页**

页面不得展示本地路径、文件名、编辑字符数量或完整扩展清单。设备行提供状态、版本、时间和处理建议；用户选择状态不标红。

- [ ] **步骤 4：验证前端**

运行：

```powershell
npm --prefix admin-web test -- --run
npm --prefix admin-web run build
```

预期：全部测试和生产构建通过，无文本溢出。

- [ ] **步骤 5：记录检查点**

Git 可用时提交：`feat: show vscode integration status`；否则更新实施日志。

### 任务 11：接入 VSIX 构建、NSIS 安装和卸载

**状态：已完成（检查点 4）**

**文件：**
- 新建：`scripts/build_vscode_extension.py`
- 新建：`native/provisioning/include/aetheris/vscode_extension.hpp`
- 新建：`native/provisioning/src/vscode_extension.cpp`
- 修改：`native/provisioning/CMakeLists.txt`
- 修改：`native/provisioning/src/plugin.cpp`
- 修改：`native/provisioning/src/exports.def`
- 修改：`native/provisioning/tests/provisioning_tests.cpp`
- 修改：`installer/windows/nsis/AetherisSetup.nsi`
- 修改：`scripts/build_windows_setup.py`
- 修改：`tests/test_nsis_build.py`
- 修改：`tests/test_nsis_script.py`

**接口：**
- 产生：`VSCodeExtensionInstaller::Install(codeExe, vsixPath) -> ExtensionOperationResult` 和 `Uninstall(codeExe, extensionId) -> ExtensionOperationResult`。
- 构建输入新增：`VSIX_FILE`、`VSIX_SHA256`。

- [ ] **步骤 1：编写原生命令和取消语义失败测试**

```cpp
FakeProcessRunner runner;
VSCodeExtensionInstaller installer(runner);
auto result = installer.Install(L"C:\\VSCode\\Code.exe", L"C:\\Aetheris\\Aetheris.vsix");
require(result.state == L"awaiting_activation", "install state mismatch");
require(runner.application == L"C:\\VSCode\\Code.exe", "must launch Code.exe directly");
require(runner.arguments == L"--install-extension \"C:\\Aetheris\\Aetheris.vsix\" --force", "wrong install args");
require(!runner.uses_shell, "shell launch forbidden");
require(DeclineVSCodeExtension().state == L"install_declined", "decline must not fail setup");
```

- [ ] **步骤 2：运行红灯**

运行：

```powershell
cmake --build build\native-provisioning --config Release
ctest --test-dir build\native-provisioning -C Release --output-on-failure
python -m unittest tests.test_nsis_build tests.test_nsis_script -v
```

预期：VSIX 参数和原生安装接口缺失而失败。

- [ ] **步骤 3：实现构建和安装**

NSIS 增加默认选中、可取消的组件页；取消写入 HKCU `Software\Aetheris\VSCodeIntegration\State=install_declined`。选中时先校验 VSIX SHA-256 和发布者，再调用 Provisioning DLL。安装失败回滚 Core 和扩展状态，不关闭 VS Code 签名验证。

- [ ] **步骤 4：验证安装包产物**

运行：

```powershell
python scripts\build_vscode_extension.py
python scripts\build_windows_core.py
python scripts\build_windows_service.py
python scripts\build_windows_setup.py
```

预期：生成 VSIX、Core、Service 和单个 NSIS EXE；所有哈希文件存在，Service Authenticode 验证通过。

- [ ] **步骤 5：记录检查点**

Git 可用时提交：`feat: package vscode extension with setup`；否则记录全部产物哈希。

### 任务 12：真实 VS Code 端到端验收和生产发布

**状态：已完成（检查点 4）**

**文件：**
- 新建：`vscode-extension/test/integration/behavior.test.ts`
- 新建：`docs/operations/vscode-extension-acceptance.md`
- 修改：`docs/operations/implementation-log.md`
- 产物：`dist/AetherisSetup-<version>.exe`

**接口：**
- 验收完整链路：VS Code → Pipe → Core SQLite → Gateway → events → clean facts → Work Episode → Admin Web。

- [ ] **步骤 1：执行真实扩展测试**

运行：`npm --prefix vscode-extension run test:integration`。

测试工作区必须在临时授权目录，依次打开、编辑、保存、切换和关闭两个测试文件；测试结束删除临时工作区。预期生成对应六类事件且不含测试文件正文。

- [ ] **步骤 2：执行隐私和进程树检查**

检查范围：扩展缓存、Core SQLite、Core 日志、HTTP 请求捕获和 PostgreSQL。使用测试正文标记 `AETHERIS_FORBIDDEN_SOURCE_SENTINEL_7F31`，所有存储和网络结果必须为零匹配。

使用 Process Explorer 或 ProcMon 验证安装和启停期间没有 `cmd.exe`、`powershell.exe`、`pwsh.exe` 子进程。

- [ ] **步骤 3：验证取消、停用、恢复和断网**

逐项验收：

```text
安装取消 -> install_declined -> 无行为事件
本地 Lens 安装 -> awaiting_activation -> active
用户停用 -> paused_by_user -> 无新事件
重新启用 -> active -> 不补采停用期间事件
Core 断网 -> 扩展缓存增长 -> Core 恢复 -> ACK 后清空
VS Code 卸载扩展 -> not_installed -> 不自动恢复
```

- [ ] **步骤 4：运行全部自动化门禁**

```powershell
python -m unittest discover -s tests -q
go -C server test ./...
npm --prefix admin-web test -- --run
npm --prefix admin-web run build
npm --prefix vscode-extension test -- --run
npm --prefix vscode-extension run typecheck
ctest --test-dir build\native-provisioning -C Release --output-on-failure
```

预期：全部退出码为 0。

- [ ] **步骤 5：服务器预检、备份和原子发布**

只读检查磁盘、服务、现有 `/opt/aetheris` 内容和当前下载哈希。生成 PostgreSQL custom-format 备份；上传到新的 staging 目录；执行 migration；原子替换 Go Server、Admin Web 和客户端安装包；保留旧二进制和安装包回滚目录。

- [ ] **步骤 6：生产验收**

验证：

```text
GET /healthz -> 200
GET /downloads/client -> 新安装包且 SHA-256 一致
设备 heartbeat -> registered
vscode_extension component_state -> active 或明确用户状态
六类 VS Code 事件 -> 服务端可见且项目归属正确
clean_event_facts -> 安全动作摘要
Work Episode -> 引用文件行为证据
服务器 /opt/aetheris 总占用 -> 小于 100GB
```

- [ ] **步骤 7：记录最终结果**

在 `docs/operations/implementation-log.md` 记录版本、哈希、备份位置、服务状态、事件样本计数、隐私扫描结果和残余限制。Git 可用时提交：`release: ship vscode behavior collection`。
