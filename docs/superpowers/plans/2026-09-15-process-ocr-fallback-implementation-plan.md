# 工作进程 OCR 保底 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为已授权但无法通过专属适配器产生数据的前台工作进程提供受限、内存化、可审计的 OCR 保底采集。

**Architecture:** Core 使用独立调度器记录每个进程最近一次有效专属事件，在连续 60 秒无数据后执行隐私门禁。通过 Win32 客户区截图、现有 Tesseract.js Worker、DLP 和 Redactor 生成低置信度 `application.activity`，并按设备内 HMAC 指纹和五分钟时间桶去重；服务端将其纳入 Forge、Work Episode 和健康诊断。

**Tech Stack:** Python 3.12、Win32 ctypes、Pillow、Tesseract.js Worker、SQLite、Go 1.23、PostgreSQL、React/TypeScript、NSIS。

**Spec:** `docs/superpowers/specs/2026-09-15-process-ocr-fallback-and-concise-rag-design.md`

## Global Constraints

- 只处理当前用户已授权且当前位于前台的工作进程。
- 连续 60 秒没有有效专属事件才进入候选；59 秒不得触发。
- Chrome、Edge、系统、安全、即时通信、凭据 UI、Aetheris 自身和隐藏窗口永久排除。
- 通用工作进程单次只截一帧，不滚动，不截整个桌面。
- 截图永不落盘；OCR 完成、超时或失败后释放所有图像和编码缓冲区。
- OCR 原文必须先通过 DLP 和 Redactor，才允许进入 SQLite。
- 每设备每分钟最多一次，每进程每五分钟最多一次；失败采用 1、2、5、10 分钟退避。
- 队列达到暂停水位或 OCR Worker 忙时跳过，不阻塞主采集循环。
- 所有界面与错误文案使用中文；日志只写固定原因码和计数。
- 工作区没有 `.git` 时不创建提交，改为更新 `docs/operations/implementation-log.md` 检查点。

---

### Task 1: OCR 候选调度器和隐私门禁

**状态：已完成（检查点 1）**

**Files:**
- Create: `src/aetheris/application_capture_policy.py`
- Test: `tests/test_application_capture_policy.py`

**Interfaces:**
- Consumes: `ConsentOutcome`、进程名、窗口状态、最近专属事件时间、队列水位和当前时间。
- Produces: `CaptureCandidate(identity_key, process_name, consent_state, project_id, foreground, visible, minimized, has_password_control, has_client_area, queue_watermark, ocr_busy, now)`。
- Produces: `ApplicationCapturePolicy.evaluate(input: CaptureCandidate) -> CaptureDecision`、`mark_native_event(identity_key, at)`、`mark_result(identity_key, result, at)`。

- [ ] **Step 1: Write the failing eligibility tests**

```python
from datetime import datetime, timedelta, timezone

def at(seconds: int) -> datetime:
    return datetime(2026, 9, 15, tzinfo=timezone.utc) + timedelta(seconds=seconds)

def candidate(**changes) -> CaptureCandidate:
    values = {
        "identity_key": "process-1", "process_name": "designer.exe", "consent_state": "allow_global",
        "project_id": "project-1", "foreground": True, "visible": True, "minimized": False,
        "has_password_control": False, "has_client_area": True, "queue_watermark": "normal",
        "ocr_busy": False, "now": at(60),
    }
    return CaptureCandidate(**{**values, **changes})

def test_candidate_requires_sixty_seconds_without_native_event():
    policy = ApplicationCapturePolicy()
    policy.mark_native_event("process-1", at(0))
    assert policy.evaluate(candidate(now=at(59))).reason_code == "native_adapter_recent"
    assert policy.evaluate(candidate(now=at(60))).eligible is True

def test_browser_password_and_unapproved_processes_are_blocked():
    assert policy.evaluate(candidate(name="chrome.exe")).reason_code == "privacy_gate_blocked"
    assert policy.evaluate(candidate(consent_state="pending")).reason_code == "privacy_gate_blocked"
    assert policy.evaluate(candidate(has_password_control=True)).reason_code == "privacy_gate_blocked"
```

- [ ] **Step 2: Run the tests and verify RED**

Run: `python -m unittest tests.test_application_capture_policy -v`  
Expected: FAIL because `application_capture_policy` does not exist.

- [ ] **Step 3: Implement deterministic policy state**

Define immutable `CaptureCandidate` and `CaptureDecision`. Enforce exact exclusions, 60-second idle threshold, foreground/visible/client-area checks, queue watermark, one-device-per-minute gate, per-process five-minute gate, and failure backoff sequence `(60, 120, 300, 600)` seconds. Store only monotonic timestamps and stable identity keys; do not store titles or OCR text in the policy.

- [ ] **Step 4: Verify GREEN and boundary cases**

Run: `python -m unittest tests.test_application_capture_policy -v`  
Expected: PASS for 59/60 seconds, exclusions, rate limits, success reset and failure backoff.

- [ ] **Step 5: Record checkpoint**

Update `docs/operations/implementation-log.md` with the policy test count and state that no production capture is active yet.

### Task 2: Win32 客户区截图和密码控件检测

**状态：已完成（检查点 1）**

**Files:**
- Create: `src/aetheris/adapters/application_window.py`
- Test: `tests/test_application_window.py`
- Modify: `src/aetheris/adapters/window.py`

**Interfaces:**
- Consumes: foreground HWND/PID from `foreground_window()` and process identity from `NativeProcessIdentityProvider`.
- Produces: `ApplicationWindowInspector.inspect() -> ApplicationWindow | None` and `ClientAreaCapture.capture(window) -> PIL.Image.Image`.

- [ ] **Step 1: Write failing native-boundary tests**

```python
def test_capture_uses_client_rect_not_virtual_desktop():
    image = capture.capture(ApplicationWindow(hwnd=42, pid=7, name="designer.exe", client_rect=(10, 20, 410, 320)))
    assert image.size == (400, 300)

def test_password_control_blocks_before_screenshot():
    window = inspector.inspect(control=password_control)
    assert window.has_password_control is True
    capture.grab.assert_not_called()
```

- [ ] **Step 2: Run the tests and verify RED**

Run: `python -m unittest tests.test_application_window -v`  
Expected: FAIL because the inspector and client-area capture do not exist.

- [ ] **Step 3: Implement direct Win32/UIA inspection**

Use `GetForegroundWindow`, `GetWindowThreadProcessId`, `IsWindowVisible`, `IsIconic`, `GetClientRect`, and `ClientToScreen`. Detect password controls with UI Automation `IsPassword` and credential window class/role allow-deny rules. Replace `tasklist` lookup in the shared foreground helper with `NativeProcessIdentityProvider.process_path()` plus executable basename so this path never launches a shell or console process.

- [ ] **Step 4: Implement bounded in-memory screenshot**

Reject zero, negative, off-screen, larger-than-16-megapixel, minimized and hidden client areas. Call `ImageGrab.grab(bbox=client_bbox, all_screens=True)` and return only the in-memory image object. Do not expose a filename argument or save method.

- [ ] **Step 5: Verify native behavior**

Run: `python -m unittest tests.test_application_window tests.test_hidden_process -v`  
Expected: PASS; tests prove client-area bounds, password gating and absence of command-line process lookup.

### Task 3: 内存 OCR、DLP、脱敏和五分钟去重

**状态：已完成（检查点 1）**

**Files:**
- Create: `src/aetheris/adapters/application_ocr.py`
- Test: `tests/test_application_ocr.py`
- Modify: `src/aetheris/command_privacy.py`

**Interfaces:**
- Consumes: `ApplicationWindow`、`ClientAreaCapture`、`OcrCapture`、`DlpMatcher`、`Redactor`、设备 HMAC key and project ID.
- Produces: `ApplicationOcrAdapter.collect(candidate) -> ApplicationCaptureResult` containing either one safe record or a fixed reason code.

- [ ] **Step 1: Write failing privacy lifecycle tests**

```python
def test_image_is_closed_after_success_and_never_written():
    result = adapter.collect(candidate)
    assert result.record["event_type"] == "application.activity"
    assert image.closed is True
    assert list(temp_directory.iterdir()) == []

def test_dlp_block_never_emits_ocr_text():
    result = adapter.collect(candidate, ocr_text="password=secret")
    assert result.reason_code == "dlp_blocked"
    assert result.record is None
    assert "secret" not in json.dumps(result.health)
```

- [ ] **Step 2: Run the tests and verify RED**

Run: `python -m unittest tests.test_application_ocr -v`  
Expected: FAIL because `ApplicationOcrAdapter` does not exist.

- [ ] **Step 3: Implement safe result projection**

Run OCR with a 30-second upper bound through the existing Worker interface. Limit OCR input to the existing image limits and output to 20,000 characters. Evaluate DLP before `Redactor.redact`; on block return `dlp_blocked`. On success produce only `application_name`, a redacted 160-character `window_context`, redacted `visible_text`, `capture_method=ocr_fallback`, `ocr_languages=[eng, chi_sim]`, and `confidence=low`.

- [ ] **Step 4: Implement HMAC fingerprint deduplication**

Use the existing device command-privacy credential to HMAC stable process identity, project ID, normalized redacted window summary, normalized redacted text digest and UTC five-minute bucket. Keep only bounded fingerprints for the last two buckets. Return `duplicate_window_content` without an event on repeat.

- [ ] **Step 5: Verify GREEN and no-disk invariant**

Run: `python -m unittest tests.test_application_ocr tests.test_ocr_runtime tests.test_ocr_stitching tests.test_dlp -v`  
Expected: PASS; temporary directory remains empty for success, block, timeout and failure.

### Task 4: Core 采集循环、授权和健康状态接入

**状态：已完成（检查点 2）**

**Files:**
- Modify: `src/aetheris/tray.py`
- Modify: `src/aetheris/adapter_health.py`
- Modify: `src/aetheris/local_view.py`
- Test: `tests/test_application_fallback_flow.py`
- Test: `tests/test_local_project_management.py`

**Interfaces:**
- Consumes: Tasks 1-3 and existing `ProcessConsentCoordinator`, `ProjectResolver`, `LocalQueue`.
- Produces: `TrayCore.capture_application_fallback(observations, native_event_keys, now) -> int` and adapter health ID `application_ocr_fallback`.

- [ ] **Step 1: Write failing end-to-end Core tests**

```python
def test_authorized_foreground_process_falls_back_after_sixty_seconds():
    first = core.capture_application_fallback([observation], set(), at(59))
    second = core.capture_application_fallback([observation], set(), at(60))
    assert first == 0
    assert second == 1
    assert queue.history()[0]["event_type"] == "application.activity"

def test_native_event_resets_fallback_timer():
    core.capture_application_fallback([observation], {identity.key}, at(60))
    assert capture.calls == 0
```

- [ ] **Step 2: Run the tests and verify RED**

Run: `python -m unittest tests.test_application_fallback_flow -v`  
Expected: FAIL because TrayCore has no fallback method.

- [ ] **Step 3: Integrate without blocking the main loop**

Collect native event identity keys during the existing adapter phase, update the scheduler, and submit at most one eligible fallback capture to a single-worker executor. Poll the completed result on later cycles, enqueue through `event_from_record`, and never wait for OCR in `capture_once`. Stop the executor and release any image during `TrayCore.stop()`.

- [ ] **Step 4: Publish health and Lens state**

Extend adapter stages with `privacy_gate`. Map counters to `discovered=eligible`, `parsed=parsed`, `skipped=skipped`, `failed=failed`; store `detected_format=ocr_fallback:<fixed-state>` only. Add a local Lens status row with enabled state, last success and counters; do not show OCR text or window titles.

- [ ] **Step 5: Verify Core and Lens**

Run: `python -m unittest tests.test_application_fallback_flow tests.test_local_project_management tests.test_tray tests.test_adapter_health tests.test_vertical_slice -v`  
Expected: PASS and no existing adapter event count changes.

### Task 5: Forge、Work Episode、检索与 Pulse 投影

**状态：已完成（检查点 2）**

**Files:**
- Modify: `server/internal/activities/classify.go`
- Modify: `server/internal/cleaning/rules.go`
- Modify: `server/internal/cleaning/repository.go`
- Modify: `server/internal/episodes/builder.go`
- Modify: `server/internal/episodes/repository.go`
- Modify: `server/internal/retrieval/document.go`
- Modify: `server/internal/effectiveness/definitions.go`
- Test: `server/internal/cleaning/rules_test.go`
- Test: `server/internal/episodes/model_test.go`
- Test: `server/internal/retrieval/document_test.go`
- Test: `server/internal/effectiveness/aggregate_test.go`

**Interfaces:**
- Consumes: `application.activity` events with low-confidence safe payload.
- Produces: `activity_type=application` facts, short Episode actions and nonduplicating Pulse activity windows.

- [ ] **Step 1: Write failing downstream tests**

```go
func TestOCRFallbackFactIsLowConfidenceHumanApplicationActivity(t *testing.T) {
    at := time.Date(2026, 9, 15, 8, 0, 0, 0, time.UTC)
    evidence := RawEvidence{EventID:"e1", TenantID:"t1", SubjectID:"s1", DeviceID:"d1", ProjectID:"p1", EventType:"application.activity", Source:"core.application.ocr", OccurredAt:at, IngestedAt:at, Payload:map[string]any{"application_name":"designer.exe", "window_context":"查看配置页面", "capture_method":"ocr_fallback"}}
    facts := NormalizeEvents([]RawEvidence{evidence}, 3)
    if facts[0].ActivityType != "application" || facts[0].ActorOrigin != "human" || facts[0].Confidence != "low" { t.Fatal(facts[0]) }
}

func TestEpisodeUsesShortOCRSummaryNotVisibleText(t *testing.T) {
    fact := Fact{EventID:"e1", SubjectID:"s1", DeviceID:"d1", ProjectID:"p1", SessionID:"app-1", EventType:"application.activity", Role:"human", Summary:"在设计工具中查看配置页面", OccurredAt:time.Now().UTC()}
    episode := Build([]Fact{fact})[0]
    if episode.Actions[0].Summary != "在设计工具中查看配置页面" { t.Fatal(episode) }
}
```

- [ ] **Step 2: Run the tests and verify RED**

Run: `go -C server test ./internal/cleaning ./internal/activities ./internal/episodes ./internal/retrieval ./internal/effectiveness -run 'Application|OCR' -v`  
Expected: FAIL because the event is not included in cleaning queries or activity types.

- [ ] **Step 3: Implement deterministic projections**

Add `application` to activity constraints and UI types. Cleaning rule version becomes 3. Store low confidence and `ocr_fallback` reason. Episode summary reads only `window_context`; Nexus document contains only “应用活动：<application_name>，<window_context>” and never `visible_text`. Pulse counts its five-minute bucket only when no accepted non-OCR fact exists for the same subject/device/project bucket.

- [ ] **Step 4: Verify downstream behavior**

Run: `go -C server test ./internal/cleaning ./internal/activities ./internal/episodes ./internal/retrieval ./internal/effectiveness -v`  
Expected: PASS with explicit nonduplication and no OCR body in Episode/Nexus tests.

- [ ] **Step 5: Record checkpoint**

Update the implementation log with cleaning rule version, migration impact and focused test output.

### Task 6: 租户总开关与管理端健康展示

**状态：已完成（检查点 3）**

**Files:**
- Create: `server/migrations/018_application_capture_policy.sql`
- Create: `server/internal/applicationpolicy/service.go`
- Create: `server/internal/applicationpolicy/service_test.go`
- Modify: `server/internal/httpapi/router.go`
- Create: `server/internal/httpapi/application_policy_handlers.go`
- Modify: `src/aetheris/gateway.py`
- Modify: `src/aetheris/tray.py`
- Modify: `admin-web/src/pages/CollectionCoveragePage.tsx`
- Modify: `admin-web/src/pages/CollectionCoveragePage.test.tsx`

**Interfaces:**
- Produces: `GET/PUT /api/v1/admin/application-capture-policy` and `GET /api/v1/device/application-capture-policy` with `{enabled, revision}`.
- Consumes: device token/admin session authorization and `application_ocr_fallback` health snapshots.

- [ ] **Step 1: Write failing policy authorization tests**

```go
func TestApplicationPolicyDefaultsEnabledAndRevisionZero(t *testing.T) {
    policy := repository.Get(ctx, "tenant-1")
    if !policy.Enabled || policy.Revision != 0 { t.Fatal(policy) }
}

func TestDeviceCannotUpdateApplicationPolicy(t *testing.T) {
    response := putAsDevice("/api/v1/admin/application-capture-policy")
    if response.Code != http.StatusUnauthorized && response.Code != http.StatusForbidden { t.Fatal(response.Code) }
}
```

- [ ] **Step 2: Run the tests and verify RED**

Run: `go -C server test ./internal/applicationpolicy ./internal/httpapi -run ApplicationPolicy -v`  
Expected: FAIL because migration, repository and routes do not exist.

- [ ] **Step 3: Implement revisioned policy and Core sync**

Create tenant-scoped policy with default enabled and monotonic revision. Audit admin changes. Core syncs after heartbeat, persists only `{enabled, revision}`, retains last valid policy on network errors and prevents new OCR tasks immediately when disabled. Existing native adapters remain active.

- [ ] **Step 4: Add product-facing health display**

Collection Coverage displays “应用 OCR 保底” with enabled/disabled, eligible, success, skipped, failed and last event. It translates fixed reason codes to Chinese and never exposes process paths, OCR text or internal exception strings.

- [ ] **Step 5: Verify policy and UI**

Run:

```powershell
go -C server test ./internal/applicationpolicy ./internal/httpapi ./internal/adapterhealth -v
python -m unittest tests.test_gateway tests.test_tray tests.test_application_fallback_flow -v
npm --prefix admin-web test -- --run
npm --prefix admin-web run build
```

Expected: all commands pass.

### Task 7: Windows 构建、真实验收和部署

**状态：已完成（检查点 4）**

**Files:**
- Modify: `src/aetheris/version.py`
- Modify: `pyproject.toml`
- Modify: version assertions under `tests/`
- Modify: `docs/operations/implementation-log.md`
- Create: `docs/operations/application-ocr-fallback-acceptance.md`
- Produce: `dist/AetherisSetup-<version>.exe`

**Interfaces:**
- Consumes: completed Core/server/Admin implementation and existing NSIS build pipeline.
- Produces: one signed-test-certificate NSIS installer and rollback-backed server release.

**Rollback:** 保留并记录上一个 Server、Migrate、迁移目录、Admin 静态目录和客户端安装包；任一部署后检查失败时立即恢复这些备份并重启服务。

- [ ] **Step 1: Run all verification before versioning**

Run:

```powershell
python -m unittest discover -s tests
go -C server test ./...
npm --prefix admin-web test -- --run
npm --prefix admin-web run build
```

Expected: zero failures.

- [ ] **Step 2: Build in dependency order**

Run Core first, then Service, sign the final Service with the configured local test certificate, verify `signtool verify /pa`, build provisioning plugin, and finally build NSIS. Do not publish if the final Service copy is unsigned.

- [ ] **Step 3: Perform controlled Windows acceptance**

Use one authorized non-browser test application and one excluded application. Verify 59/60-second boundary, no temporary images, one event per five-minute bucket, Lens counters, server health snapshot, Forge fact and Work Episode summary. Verify direct native event resets the timer.

- [ ] **Step 4: Deploy with backup and health checks**

Read `/opt/aetheris` and disk usage first. Back up server binary, migrations, Admin static files and previous installer. Deploy migration/server/Admin, restart `aetheris-server`, then publish the installer. Verify `/healthz`, `/admin/`, `/downloads/client`, file length and SHA-256. Keep total `/opt/aetheris` usage below 100GB.

- [ ] **Step 5: Record final evidence**

Write exact test counts, installer hash, server backup directory and real acceptance results to both acceptance and implementation logs.
