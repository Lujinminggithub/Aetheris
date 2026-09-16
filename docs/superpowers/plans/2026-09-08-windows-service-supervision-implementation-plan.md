# Aetheris Core Windows Service Supervision Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver Aetheris Core 0.4.8 with a LocalSystem Windows service that starts and supervises the Core tray process in the current active console user's session, survives Core and service crashes, preserves explicit user exit semantics, and installs without Task Scheduler or shell helpers.

**Architecture:** A dedicated native `AetherisCoreService.exe` owns SCM lifecycle, WTS session discovery, user-token process launch, recovery state, and a constrained named-pipe protocol. The existing Python/PyInstaller `AetherisCore.exe` remains the non-elevated collection and tray process. The NSIS provisioning plugin installs and updates the service through native SCM APIs while preserving Core data and DPAPI credentials.

**Tech Stack:** C++17/MSVC Win32 Service API, WTS API, Userenv, named pipes, CMake/CTest, Python 3.11 `ctypes`, PyInstaller windowed, NSIS 3.12 Unicode/MUI2, Go 1.22 server, PostgreSQL-backed Gateway.

**Spec:** `docs/superpowers/specs/2026-09-08-windows-service-supervision-design.md`

## Global Constraints

- Target release is 0.4.8 for Windows 10/11 x64.
- Local Beta service binaries must pass `signtool verify /pa` and use a code-signing certificate trusted in LocalMachine Root and TrustedPublisher. `/kp` is not required for this user-mode service. Public releases still require a production Authenticode certificate.
- `AetherisCoreService` runs as LocalSystem with Automatic Delayed Start; `AetherisCore.exe` runs with the active console user's non-elevated token.
- The first release supervises only `WTSGetActiveConsoleSessionId()` and ignores parallel RDP sessions.
- Service binaries live below `%ProgramFiles%\Aetheris\Service`; service state and logs live below `%ProgramData%\Aetheris\Service`.
- Core, config, DPAPI credentials, SQLite, logs, and local Lens remain under the user-selected Core directory.
- No install or runtime path may invoke `cmd.exe`, PowerShell, `sc.exe`, `schtasks.exe`, a batch file, or a PowerShell script.
- The service never opens device/control credentials, SQLite, captured text, OCR content, screenshots, project files, or network sockets.
- IPC is `\\.\pipe\Aetheris.Core.Service.v1`, length-prefixed UTF-8 JSON, maximum 16 KiB, and authenticated by pipe ACL plus client PID/SID/SessionId.
- Unexpected Core exits restart after 60 seconds; five failures in ten minutes trigger a 15-minute backoff; thirty healthy minutes reset failures.
- Authenticated `normal_exit` followed by exit code 0 within ten seconds suppresses restarts until logoff or Windows reboot.
- Installer failure must preserve config, credentials, SQLite, project authorization, and unknown files.
- The workspace has no `.git` metadata. Each task uses passing tests, file hashes, and the implementation log as its checkpoint; no task runs a Git commit command.
- Server commands never contain the previously supplied root password. SSH password entry is interactive only.

## File Responsibility Map

- `native/core-service/include/aetheris/service_config.hpp`: validated HKLM/service paths and immutable service configuration.
- `native/core-service/include/aetheris/recovery_policy.hpp`: pure Core restart and suppression state machine.
- `native/core-service/include/aetheris/session_launcher.hpp`: active-console discovery and user-token Core launch.
- `native/core-service/include/aetheris/ipc_protocol.hpp`: bounded message parsing, serialization, and client identity model.
- `native/core-service/include/aetheris/service_host.hpp`: SCM control handling and orchestration interfaces.
- `native/core-service/src/*.cpp`: Win32 implementations; each source mirrors one header responsibility.
- `native/core-service/src/main.cpp`: service dispatcher entry only.
- `native/core-service/tests/service_tests.cpp`: deterministic unit tests with fake clock/session/process APIs.
- `native/core-service/tests/test_agent.cpp`: disposable user-process fixture for elevated service integration tests.
- `native/provisioning/include/aetheris/core_service_install.hpp`: SCM installation/query/removal contract used by NSIS.
- `native/provisioning/src/core_service_install.cpp`: elevated SCM and protected registry operations only.
- `src/aetheris/service_ipc.py`: user-mode named-pipe client used by Core and local Lens.
- `installer/windows/nsis/AetherisSetup.nsi`: staging, identity reuse, service install, rollback, shortcuts, and uninstall UI.
- `scripts/build_windows_service.py`: reproducible x64 service build.
- `scripts/build_windows_setup.py`: supplies Core, service, and provisioning plugin to NSIS.

---

### Task 1: Service Configuration And Recovery State Machine

**Files:**
- Create: `native/core-service/CMakeLists.txt`
- Create: `native/core-service/include/aetheris/service_config.hpp`
- Create: `native/core-service/src/service_config.cpp`
- Create: `native/core-service/include/aetheris/recovery_policy.hpp`
- Create: `native/core-service/src/recovery_policy.cpp`
- Create: `native/core-service/tests/service_tests.cpp`

**Interfaces:**
- Produces: `ServiceConfig load_service_config(RegistryReader&)`.
- Produces: `RecoveryPolicy::on_ready`, `on_heartbeat`, `on_authenticated_normal_exit`, `on_process_exit`, `on_logoff`, and `on_boot_change`.
- Produces: `RecoveryDecision { SupervisorState state; bool launch; TimePoint launch_at; }` for Task 4.

- [x] **Step 1: Write configuration and recovery tests before implementation**

```cpp
TEST_CASE("config rejects relative or missing core path") {
    FakeRegistry registry{{L"CoreRoot", L"..\\Core"}};
    REQUIRE_THROWS_AS(load_service_config(registry), ConfigError);
}

TEST_CASE("five failures in ten minutes enter backoff") {
    FakeClock clock;
    RecoveryPolicy policy(clock, 60s, 5, 10min, 15min, 30min);
    for (int i = 0; i < 5; ++i) {
        policy.on_process_exit(0xc0000005, false);
        clock.advance(61s);
    }
    REQUIRE(policy.decision().state == SupervisorState::crash_loop_backoff);
    REQUIRE(policy.decision().launch_at == clock.now() + 15min);
}

TEST_CASE("normal exit survives service restart but clears on logoff") {
    RecoveryPolicy policy(clock, persisted_state);
    policy.on_authenticated_normal_exit(session);
    REQUIRE(policy.decision().state == SupervisorState::user_suppressed);
    RecoveryPolicy restored(clock, policy.serialize());
    REQUIRE(restored.decision().state == SupervisorState::user_suppressed);
    restored.on_logoff(session);
    REQUIRE(restored.decision().launch);
}
```

- [x] **Step 2: Configure and run the unit target to prove tests fail**

Run: `cmake -S native/core-service -B build/core-service -A x64`

Run: `cmake --build build/core-service --config Release`

Run: `ctest --test-dir build/core-service -C Release --output-on-failure`

Expected: compilation fails because `ServiceConfig` and `RecoveryPolicy` are not defined.

- [x] **Step 3: Implement strict configuration and pure recovery types**

```cpp
enum class SupervisorState { waiting_for_user, starting, running, restart_wait, crash_loop_backoff, user_suppressed };

struct ServiceConfig {
    std::filesystem::path core_root;
    std::filesystem::path core_executable;
    std::filesystem::path config_file;
    std::wstring install_user_sid;
    std::wstring expected_version;
};

struct RecoveryDecision {
    SupervisorState state = SupervisorState::waiting_for_user;
    bool launch = false;
    std::chrono::steady_clock::time_point launch_at{};
};
```

`load_service_config` must read `HKLM\Software\Aetheris\Core` with `KEY_WOW64_64KEY`, require an absolute existing Core root, derive `AetherisCore.exe` and `config\aetheris.json` itself, reject embedded NULs, and never accept executable arguments from the registry.

- [x] **Step 4: Implement deterministic state serialization**

Persist only `schema_version`, hashed user identity, SessionId, boot-cycle identifier, suppression time, and failure timestamps. Compute `boot_epoch_100ns` as current UTC FILETIME minus `GetTickCount64() * 10000`; a restored value matches the current boot only within a five-second tolerance. Write to `%ProgramData%\Aetheris\Service\state.json.tmp`, call `FlushFileBuffers`, then replace `state.json` with `MoveFileExW(MOVEFILE_REPLACE_EXISTING | MOVEFILE_WRITE_THROUGH)`. Reject files over 64 KiB and reset to an empty safe state on parse failure.

- [x] **Step 5: Run focused tests and record a checkpoint**

Run: `ctest --test-dir build/core-service -C Release --output-on-failure`

Run: `Get-FileHash native\core-service\src\recovery_policy.cpp,native\core-service\src\service_config.cpp -Algorithm SHA256`

Expected: all unit tests pass; no registry value or persisted state can introduce a launch command.

---

### Task 2: Active Console Session Launch And Process Ownership

**Files:**
- Create: `native/core-service/include/aetheris/session_launcher.hpp`
- Create: `native/core-service/src/session_launcher.cpp`
- Modify: `native/core-service/tests/service_tests.cpp`
- Modify: `native/core-service/CMakeLists.txt`

**Interfaces:**
- Consumes: `ServiceConfig` from Task 1.
- Produces: `SessionInfo active_console_session(SessionApi&)`.
- Produces: `LaunchResult launch_core_for_session(const ServiceConfig&, const SessionInfo&, SessionApi&)`.
- Produces: `bool process_matches_expected_user(HANDLE, const SessionInfo&, const ServiceConfig&)`.

- [x] **Step 1: Add failing tests for console-only selection and token use**

```cpp
TEST_CASE("RDP session is ignored when console user is absent") {
    FakeSessionApi api({Session{4, RdpActive, sid_a}});
    REQUIRE(active_console_session(api).state == SessionState::none);
}

TEST_CASE("Core launches with duplicated primary user token") {
    FakeSessionApi api({Session{2, ConsoleActive, sid_a}});
    auto result = launch_core_for_session(config, active_console_session(api), api);
    REQUIRE(result.started);
    REQUIRE(api.last_creation_flags == (CREATE_UNICODE_ENVIRONMENT | CREATE_NO_WINDOW));
    REQUIRE(api.last_application == config.core_executable);
    REQUIRE(api.last_arguments == L"--config \"D:\\Aetheris\\config\\aetheris.json\" --supervised --service-session 2");
    REQUIRE(api.last_desktop == L"winsta0\\default");
}
```

- [x] **Step 2: Run the focused test and verify the missing implementation failure**

Run: `cmake --build build/core-service --config Release`

Run: `ctest --test-dir build/core-service -C Release --output-on-failure`

Expected: compilation fails on `active_console_session` and `launch_core_for_session`.

- [x] **Step 3: Implement WTS discovery and SID validation**

Use `WTSGetActiveConsoleSessionId`, `WTSQuerySessionInformationW(WTSConnectState)`, `WTSQueryUserToken`, `GetTokenInformation(TokenUser)`, and constant-time SID equality through `EqualSid`. Return `SessionState::none` for `0xffffffff`, RDP-only sessions, empty user SID, or a SID different from `ServiceConfig.install_user_sid`.

- [x] **Step 4: Implement non-elevated Core launch**

```cpp
HANDLE user_token = query_user_token(session.id);
HANDLE primary_token = duplicate_primary_token(user_token);
void* environment = create_environment_block(primary_token);
CreateProcessAsUserW(
    primary_token,
    config.core_executable.c_str(),
    mutable_command_line.data(),
    nullptr, nullptr, FALSE,
    CREATE_UNICODE_ENVIRONMENT | CREATE_NO_WINDOW,
    environment,
    config.core_root.c_str(),
    &startup, &process);
```

Close every token/thread/process handle on all paths. Set `STARTUPINFO.lpDesktop` to the fixed literal `winsta0\\default` so the tray enters the interactive desktop, but never accept a desktop value from configuration or IPC. Do not use an elevated token; verify the returned process SessionId and user SID before accepting it.

- [x] **Step 5: Run handle-leak and argument tests**

Run: `ctest --test-dir build/core-service -C Release --output-on-failure`

Expected: console selection, RDP exclusion, exact application/argument separation, SID mismatch, environment cleanup, and handle cleanup tests pass.

---

### Task 3: Authenticated Named-Pipe IPC And Service State Log

**Files:**
- Create: `native/core-service/include/aetheris/ipc_protocol.hpp`
- Create: `native/core-service/src/ipc_protocol.cpp`
- Create: `native/core-service/include/aetheris/ipc_server.hpp`
- Create: `native/core-service/src/ipc_server.cpp`
- Create: `native/core-service/include/aetheris/service_log.hpp`
- Create: `native/core-service/src/service_log.cpp`
- Modify: `native/core-service/tests/service_tests.cpp`
- Modify: `native/core-service/CMakeLists.txt`

**Interfaces:**
- Consumes: expected user SID and SessionId from Tasks 1-2.
- Produces: `parse_ipc_message(std::string_view) -> IpcMessage`.
- Produces: `IpcServer::start`, `stop`, and callback delivery for fixed message types.
- Produces: `ServiceLog::write(EventCode, ServiceLogFields)` with 1 MiB/5-file rotation.

- [x] **Step 1: Add failing protocol and identity tests**

```cpp
TEST_CASE("IPC rejects oversized and unknown messages") {
    REQUIRE_THROWS(parse_ipc_frame(std::string(16 * 1024 + 1, 'x')));
    REQUIRE_THROWS(parse_ipc_message(R"({"version":1,"type":"run_path","pid":7,"session_id":2})"));
}

TEST_CASE("pipe client must match pid sid and session") {
    FakePipeClient client{42, sid_a, 2};
    REQUIRE(authenticate_pipe_client(client, ExpectedClient{42, sid_a, 2}));
    client.session_id = 3;
    REQUIRE_FALSE(authenticate_pipe_client(client, ExpectedClient{42, sid_a, 2}));
}
```

- [x] **Step 2: Run tests and confirm protocol symbols are missing**

Run: `cmake --build build/core-service --config Release`

Expected: compilation fails on protocol and pipe server symbols.

- [x] **Step 3: Implement bounded framing and a closed message enum**

```cpp
enum class IpcType { ready, heartbeat, normal_exit, resume, status_request, status_response, prepare_update, update_ready, session_stop };

struct IpcMessage {
    int version = 1;
    IpcType type;
    DWORD pid = 0;
    DWORD session_id = 0;
    std::uint64_t monotonic_ms = 0;
};
```

Parse only required scalar fields, reject duplicates, nesting, trailing bytes, invalid UTF-8, values outside Win32 integer ranges, and frames larger than 16 KiB. Serialize `status_response` from service-owned fields; never echo arbitrary client input.

- [x] **Step 4: Implement pipe ACL and client-token authentication**

Build an SDDL descriptor granting full access to SYSTEM and Administrators and read/write to the exact install user SID. After `ConnectNamedPipe`, call `GetNamedPipeClientProcessId`, open the client process with `PROCESS_QUERY_LIMITED_INFORMATION`, and compare token SID and SessionId before reading a frame.

- [x] **Step 5: Implement privacy-bounded logging**

Allowed log keys are `event_code`, `service_version`, `session_id`, `core_pid`, `exit_code`, `attempt`, and `next_retry_utc`. Hash SID values before logging and reject all free-form strings except the fixed `EventCode` enum.

- [x] **Step 6: Run IPC, ACL, fuzz-boundary, and log leakage tests**

Run: `ctest --test-dir build/core-service -C Release --output-on-failure`

Expected: malformed frames, forged PID/SID/session, oversized data, and sensitive log values are rejected; rotation keeps at most six files including the active log.

---

### Task 4: Windows Service Host And End-To-End Supervisor Loop

**Files:**
- Create: `native/core-service/include/aetheris/service_host.hpp`
- Create: `native/core-service/src/service_host.cpp`
- Create: `native/core-service/src/main.cpp`
- Create: `native/core-service/tests/test_agent.cpp`
- Modify: `native/core-service/tests/service_tests.cpp`
- Modify: `native/core-service/CMakeLists.txt`

**Interfaces:**
- Consumes: Task 1 recovery/config, Task 2 session launcher, Task 3 IPC/logging.
- Produces: `CoreSupervisor::run`, `on_service_control`, `on_session_change`, `on_core_exit`, and `on_ipc_message`.
- Produces: GUI-subsystem `AetherisCoreService.exe` with no console window.

- [ ] **Step 1: Write failing orchestration tests with fake APIs**

```cpp
TEST_CASE("service starts one Core for active console user") {
    CoreSupervisor supervisor(config, fake_sessions, fake_processes, fake_ipc, clock, state_store);
    supervisor.on_service_start();
    REQUIRE(fake_processes.launch_count == 1);
    supervisor.on_session_change(WTS_SESSION_UNLOCK, active_session);
    REQUIRE(fake_processes.launch_count == 1);
}

TEST_CASE("authenticated normal exit suppresses restart") {
    supervisor.on_ipc_message(normal_exit_message(core_pid, session_id));
    supervisor.on_core_exit(core_pid, 0, clock.now() + 2s);
    clock.advance(2min);
    supervisor.tick();
    REQUIRE(fake_processes.launch_count == 1);
}
```

- [ ] **Step 2: Verify tests fail before the service host exists**

Run: `cmake --build build/core-service --config Release`

Expected: compilation fails on `CoreSupervisor` and service entry symbols.

- [x] **Step 3: Implement SCM dispatcher and service status transitions**

```cpp
int WINAPI wWinMain(HINSTANCE, HINSTANCE, PWSTR, int) {
    SERVICE_TABLE_ENTRYW table[] = {
        {const_cast<wchar_t*>(L"AetherisCoreService"), service_main},
        {nullptr, nullptr},
    };
    return StartServiceCtrlDispatcherW(table) ? 0 : static_cast<int>(GetLastError());
}
```

Report `START_PENDING`, `RUNNING`, `STOP_PENDING`, and `STOPPED` with valid checkpoints and wait hints. Register `SERVICE_ACCEPT_STOP | SERVICE_ACCEPT_SHUTDOWN | SERVICE_ACCEPT_SESSIONCHANGE` through `RegisterServiceCtrlHandlerExW`.

- [ ] **Step 4: Implement the supervisor loop**

The loop owns exactly one active-console `PROCESS_INFORMATION`, one wait registration, one recovery timer, and one IPC server. `session_stop` and `prepare_update` get ten seconds for graceful exit; forced termination is allowed only for the exact tracked PID and user token. Unknown same-name processes are never terminated.

- [x] **Step 5: Configure service self-recovery metadata support**

Expose constants used by the installer: service name `AetherisCoreService`, start type `SERVICE_AUTO_START`, delayed auto start enabled, failure reset period `86400`, and three `SC_ACTION_RESTART` entries with 60,000 ms delay.

- [x] **Step 6: Build and inspect the service binary**

Run: `cmake --build build/core-service --config Release`

Run: `dumpbin.exe /headers /imports build\core-service\Release\AetherisCoreService.exe`

Expected: x64 GUI subsystem binary imports Advapi32, Wtsapi32, Userenv, Kernel32, and no shell executable; all CTest tests pass.

---

### Task 5: Core User-Agent IPC, Resume, And Normal Exit

**Files:**
- Create: `src/aetheris/service_ipc.py`
- Create: `tests/test_service_ipc.py`
- Modify: `src/aetheris/tray.py`
- Modify: `src/aetheris/lifecycle.py`
- Modify: `src/aetheris/local_view.py`
- Modify: `tests/test_lifecycle.py`
- Modify: `tests/test_local_view.py`
- Modify: `tests/test_tray.py`

**Interfaces:**
- Consumes: pipe and message contract from Task 3.
- Produces: `ServiceClient.connect`, `ready`, `heartbeat`, `normal_exit`, `resume`, `status`, and `close`.
- Produces: `parse_args(...).service_session: int | None`.
- Produces: `run_tray(config_path, service_client=None)` with authenticated exit notification.

- [x] **Step 1: Write failing Python protocol and tray-exit tests**

```python
def test_service_frame_is_length_prefixed_and_bounded():
    transport = FakePipeTransport()
    client = ServiceClient(transport, pid=42, session_id=2, clock=lambda: 100)
    client.ready()
    body = json.loads(transport.frames[0][4:])
    assert body == {"version": 1, "type": "ready", "pid": 42, "session_id": 2, "monotonic_ms": 100}

def test_tray_exit_notifies_service_before_stopping():
    calls = []
    run_exit_handler(FakeServiceClient(calls), FakeTrayApp(calls))
    assert calls == ["normal_exit", "app_stop", "icon_stop"]
```

- [x] **Step 2: Run focused tests and confirm failure**

Run: `python -m unittest tests.test_service_ipc tests.test_lifecycle tests.test_tray -v`

Expected: import or assertion failure because `ServiceClient` and `--service-session` do not exist.

- [x] **Step 3: Implement the Windows named-pipe client without pywin32**

Use `ctypes.WinDLL("kernel32", use_last_error=True)` with `CreateFileW`, `ReadFile`, `WriteFile`, `WaitNamedPipeW`, and `CloseHandle`. Encode deterministic ASCII-safe JSON, prepend a little-endian uint32 length, reject responses over 16 KiB, and reconnect with bounded backoff. Never include config content or credential values.

- [x] **Step 4: Wire lifecycle messages into the tray process**

```python
parser.add_argument("--service-session", type=int)

client = ServiceClient.for_current_process(args.service_session) if args.service_session is not None else None
if client:
    client.connect(timeout_seconds=10)
    client.ready()

def exit_app(icon, _item):
    if client:
        client.normal_exit()
    app.stop()
    icon.stop()
```

Send heartbeat every 30 seconds while the tray worker is healthy. A manually started Core derives its own SessionId with `ProcessIdToSessionId`, connects without `--service-session`, sends `resume`, then `ready`. If the service pipe is absent, a packaged Core shows `Aetheris Core Service 未运行` and returns 1. Source-tree diagnostics may run without the service only when the explicit `--standalone` flag is present; NSIS and Start Menu shortcuts never set that flag.

- [x] **Step 5: Preserve abnormal-exit semantics**

Top-level exceptions must still return 1 and must not send `normal_exit`. `already_running` returns 0 without changing service suppression. `session_stop` and `prepare_update` return 0 with distinct fixed lifecycle reasons but do not set user suppression.

- [ ] **Step 6: Surface service status in local Lens**

Pass `ServiceClient.status` into `create_local_view_server`. Add a read-only “Core 服务” section showing only service state, Core PID, last heartbeat, last exit classification, next retry time, and backoff reason. The existing protected local action for “退出 Core” sends `normal_exit`; “启动 Core” is exposed only when the service response is `user_suppressed` and sends `resume`. Do not render the service account, full SID, service path, or Core command line.

- [x] **Step 7: Run Python tests and sensitive-data scans**

Run: `python -m unittest tests.test_service_ipc tests.test_lifecycle tests.test_tray tests.test_single_instance -v`

Run: `python -m unittest tests.test_no_sensitive_literals tests.test_command_cleaning -v`

Expected: all pass; pipe frames and lifecycle logs contain no gateway token, credential plaintext, project path, OCR text, or event payload.

---

### Task 6: Native SCM Installer And Existing-Identity Reuse

**Files:**
- Create: `native/provisioning/include/aetheris/core_service_install.hpp`
- Create: `native/provisioning/src/core_service_install.cpp`
- Modify: `native/provisioning/include/aetheris/provision.hpp`
- Modify: `native/provisioning/src/provision.cpp`
- Modify: `native/provisioning/src/plugin.cpp`
- Modify: `native/provisioning/src/exports.def`
- Modify: `native/provisioning/CMakeLists.txt`
- Modify: `native/provisioning/tests/provisioning_tests.cpp`
- Modify: `tests/test_native_plugin.py`
- Delete after replacement tests pass: `native/provisioning/include/aetheris/startup_task.hpp`
- Delete after replacement tests pass: `native/provisioning/src/startup_task.cpp`

**Interfaces:**
- Consumes: built service executable and user Core root.
- Produces: `ServiceResult install_core_service(const ServiceInstallRequest&)`.
- Produces: `stop_core_service`, `query_core_service`, `start_core_service`, and `remove_core_service`.
- Produces: `ExistingDeviceResult verify_existing_device(const path&, const wstring& gateway, HttpTransport)`.
- Produces NSIS exports: `VerifyExistingDevice`, `InstallCoreService`, `QueryCoreService`, `StartCoreService`, `StopCoreService`, `RemoveCoreService`.

- [x] **Step 1: Add failing SCM and identity-reuse tests**

```cpp
TEST_CASE("service request fixes LocalSystem and derived command") {
    ServiceInstallRequest request{service_exe, core_root, user_sid, L"0.4.8"};
    auto spec = core_service_spec(request);
    REQUIRE(spec.account == L"LocalSystem");
    REQUIRE(spec.start_type == SERVICE_AUTO_START);
    REQUIRE(spec.binary_path == service_exe);
    REQUIRE(spec.core_command.empty());
}

TEST_CASE("existing device heartbeat reuses encrypted identity") {
    auto result = verify_existing_device(root, L"http://127.0.0.1:18080", success_transport);
    REQUIRE(result.state == ExistingDeviceState::reusable);
    REQUIRE(result.device_id == expected_device);
}
```

- [x] **Step 2: Run native tests and verify new interfaces are absent**

Run: `python scripts\build_provisioning_plugin.py`

Run: `ctest --test-dir build/native-provisioning -C Release --output-on-failure`

Expected: compile failure for missing SCM and existing-device interfaces.

- [x] **Step 3: Implement idempotent SCM installation with native APIs**

```cpp
SC_HANDLE manager = OpenSCManagerW(nullptr, nullptr, SC_MANAGER_CONNECT | SC_MANAGER_CREATE_SERVICE);
SC_HANDLE service = CreateServiceW(
    manager, L"AetherisCoreService", L"Aetheris Core Service",
    SERVICE_QUERY_STATUS | SERVICE_START | SERVICE_STOP | SERVICE_CHANGE_CONFIG | DELETE,
    SERVICE_WIN32_OWN_PROCESS, SERVICE_AUTO_START, SERVICE_ERROR_NORMAL,
    service_executable.c_str(), nullptr, nullptr, nullptr, L"LocalSystem", nullptr);
```

On `ERROR_SERVICE_EXISTS`, open and update the same service with `ChangeServiceConfigW`. Apply `SERVICE_DELAYED_AUTO_START_INFO`, `SERVICE_FAILURE_ACTIONS_FLAG`, and the exact three restart actions from the spec. Write HKLM config with `KEY_WOW64_64KEY`; never store a token or Core command line.

Create `%ProgramData%\Aetheris\Service` and apply an explicit protected DACL granting full control only to SYSTEM and Administrators. Verify the DACL after writing; do not continue service installation if authenticated users retain write access.

- [x] **Step 4: Implement bounded start/stop/query/remove operations**

Poll `QueryServiceStatusEx` at 250 ms intervals for at most 30 seconds. Return fixed errors such as `service_start_timeout`, `service_stop_timeout`, and `service_delete_failed_<win32>`. Treat an already-running, already-stopped, or already-deleted service as success.

- [x] **Step 5: Implement existing-device verification**

Read `aetheris.json`, DPAPI-decrypt `device.credential` in memory, POST `/api/v1/device/heartbeat`, require `status=online` and exact tenant/subject/device identity, then zero plaintext. Return only `reusable`, `missing`, `credential_invalid`, or `server_unavailable`; never return token or raw response.

- [x] **Step 6: Replace plugin exports and remove task code**

Remove `InstallStartupTask`, `RemoveStartupTask`, `QueryStartupTask`, and `RunStartupTask` from source and export tests. Delete Task Scheduler libraries from `target_link_libraries` after `startup_task.cpp` is removed. Add Advapi32 for SCM operations.

- [x] **Step 7: Run native and export verification**

Run: `python scripts\build_provisioning_plugin.py`

Run: `ctest --test-dir build/native-provisioning -C Release --output-on-failure`

Run: `python -m unittest tests.test_native_plugin -v`

Expected: all pass; dumpbin contains all six service exports and none of the four task exports.

---

### Task 7: NSIS Service Installation, Upgrade Rollback, And Uninstall

**Files:**
- Modify: `installer/windows/nsis/AetherisSetup.nsi`
- Modify: `installer/windows/nsis/pages/Enrollment.nsh`
- Modify: `scripts/build_windows_setup.py`
- Modify: `tests/test_nsis_build.py`
- Modify: `tests/test_nsis_script.py`
- Delete: `installer/windows/nsis/AetherisStartupRepair.nsi`
- Delete: `tests/test_startup_repair_script.py`

**Interfaces:**
- Consumes: Core EXE, service EXE, and provisioning DLL.
- Produces: one `AetherisSetup-0.4.8.exe` with one UAC and native service lifecycle.
- Produces: rollback labels that restore `.previous` service/Core files and previous HKLM values.

- [x] **Step 1: Update build-command tests before the script**

```python
command = build_command(
    makensis, script, version="0.4.8", core=core,
    service=service, plugin=plugin, output_dir=dist,
    gateway_url="http://192.168.78.138:8080",
)
self.assertIn(f"/DSERVICE_EXE={service}", command)
```

Add source assertions for `InstallCoreService`, `QueryCoreService`, `StopCoreService`, `RemoveCoreService`, `$PROGRAMFILES64\Aetheris\Service`, `.previous`, and `CreateShortCut`. Assert task exports, `schtasks`, `cmd.exe`, PowerShell, and Startup Repair are absent.

- [x] **Step 2: Run NSIS tests and confirm they fail**

Run: `python -m unittest tests.test_nsis_build tests.test_nsis_script -v`

Expected: failures for missing service define and obsolete task calls.

- [x] **Step 3: Add existing-device reuse before enrollment UI**

In `EnrollmentCreate`, call `VerifyExistingDevice` after `$INSTDIR` is known. On `reusable`, set `$ReuseExistingDevice=1` and `Abort` the custom page so no enrollment prompt appears. On `server_unavailable`, show Retry/Cancel and repeat verification without displaying or accepting an enrollment code. Only `missing` and `credential_invalid` display the enrollment input.

- [ ] **Step 4: Implement same-volume staging and backup**

Use `$INSTDIR\.installing\0.4.8` for Core staging and `$PROGRAMFILES64\Aetheris\Service\.installing` for service staging. Before replacement, stop the existing service and rename owned files to `.previous`. Never delete config, credential, data, logs, project settings, or unknown files during this phase.

- [ ] **Step 5: Install and verify the service**

```nsis
Push "$PROGRAMFILES64\Aetheris\Service\AetherisCoreService.exe"
Push "$INSTDIR"
Push "${VERSION}"
AetherisProvisioning::InstallCoreService /NOUNLOAD
Pop $0
StrCmp $0 "ok" service_installed service_install_failed
```

Start the service, poll `QueryCoreService` for `running`, then poll `VerifyCoreStatus` for at most 60 seconds. Only then delete `.previous`, write the uninstaller, and show the finish page.

- [ ] **Step 6: Implement rollback labels with no recursive broad delete**

On service/Core verification failure: stop the new service, restore old service/Core files and prior HKLM configuration, reinstall/start the old service if it existed, remove only the exact staging files, and display a stable Chinese error. Verify config and credential hashes are unchanged in the test fixture.

- [ ] **Step 7: Implement Start Menu resume and safe uninstall**

Create an “Aetheris Core” shortcut targeting the user Core executable and an uninstall shortcut. Uninstall requests Core exit, stops and removes the service, revokes the device, removes service binaries only after SCM deletion succeeds, deletes credentials, and leaves data/config/logs unless the user selects the explicit data-removal checkbox.

- [x] **Step 8: Compile installer fixtures and run script tests**

Run: `python -m unittest tests.test_nsis_build tests.test_nsis_script tests.test_native_plugin -v`

Expected: NSIS compiles with fixture EXEs; static checks find no task/shell path and confirm rollback labels occur before success.

---

### Task 8: Version 0.4.8 Build And Local Elevated Acceptance

**Files:**
- Create: `scripts/build_windows_service.py`
- Create: `tests/test_windows_service_build.py`
- Modify: `scripts/build_windows_core.py`
- Modify: `scripts/build_windows_setup.py`
- Modify: `src/aetheris/version.py`
- Modify: `pyproject.toml`
- Modify: `admin-web/package.json`
- Modify: `README.md`
- Modify: `installer/windows/README_INSTALL.md`
- Modify: `deploy/.env.example`
- Modify version fixtures in: `tests/test_bootstrap.py`, `tests/test_downloads.py`, `tests/test_executable_entry.py`, `tests/test_nsis_build.py`, `tests/test_nsis_script.py`, `tests/test_setup_build.py`, `tests/test_tray.py`

**Interfaces:**
- Produces: `dist/AetherisCoreService.exe`.
- Produces: `dist/AetherisCore-0.4.8.exe`.
- Produces: `dist/AetherisSetup-0.4.8.exe` and SHA-256 sidecars.

- [x] **Step 1: Add failing build tests for all three binaries and one version**

```python
self.assertEqual(__version__, "0.4.8")
self.assertTrue((dist / "AetherisCoreService.exe").is_file())
self.assertTrue((dist / "AetherisCore-0.4.8.exe").is_file())
self.assertTrue((dist / "AetherisSetup-0.4.8.exe").is_file())
```

- [x] **Step 2: Run version/build tests and confirm 0.4.7 failures**

Run: `python -m unittest tests.test_windows_service_build tests.test_nsis_build tests.test_setup_build tests.test_bootstrap -v`

Expected: version and missing service artifact failures.

- [x] **Step 3: Implement the service build script**

```python
subprocess.run([cmake, "-S", str(SOURCE), "-B", str(BUILD), "-A", "x64"], check=True)
subprocess.run([cmake, "--build", str(BUILD), "--config", "Release"], check=True)
shutil.copy2(BUILD / "Release" / "AetherisCoreService.exe", DIST / "AetherisCoreService.exe")
```

Build scripts must fail if node/tesseract runtime, Core, service, plugin, or NSIS is missing. Generate SHA-256 files with ASCII content and no secrets.

- [x] **Step 4: Update the repository version atomically to 0.4.8**

Set Python package, Admin Web package, README, installer docs, deploy example, and every release fixture to `0.4.8`. Do not change the Model Gateway's independent `0.1.0` package version.

- [x] **Step 5: Run the complete pre-build test matrix serially**

Run: `python -m unittest discover -s tests -v`

Run: `cmake --build build/core-service --config Release; ctest --test-dir build/core-service -C Release --output-on-failure`

Run: `python scripts\build_provisioning_plugin.py; ctest --test-dir build/native-provisioning -C Release --output-on-failure`

Run from `server`: `go test ./...`

Run from `server`: `go vet ./...`

Run from `admin-web`: `npm test -- --run`

Run from `admin-web`: `npm run build`

- [x] **Step 6: Build release artifacts serially**

Run: `python scripts\build_windows_service.py`

Run: `python scripts\build_windows_core.py`

Run: `python scripts\build_windows_setup.py`

Run: `Get-FileHash dist\AetherisCoreService.exe,dist\AetherisCore-0.4.8.exe,dist\AetherisSetup-0.4.8.exe -Algorithm SHA256`

Run: `signtool verify /pa /v dist\AetherisCoreService.exe`

Expected: `/pa` passes before local service installation. Do not use `/kp`, which validates kernel-mode signing policy rather than this user-mode service.

- [ ] **Step 7: Run one elevated local installation with UAC confirmation**

Run the Setup through Windows UI, select `D:\FbBrowser\Aetheris`, and verify it reuses the existing credential when heartbeat succeeds. The user confirms the single UAC at action time. Do not send enrollment or device credentials through chat, commands, or logs.

- [ ] **Step 8: Verify service, user Core, tray, and process tree**

Run read-only SCM/API checks to confirm `AetherisCoreService` is Running, delayed auto-start is enabled, recovery actions are 60 seconds, the service executable is under Program Files, and Core runs in the active console SessionId with Medium integrity. Confirm the process tree contains no cmd, PowerShell, `sc.exe`, or `schtasks.exe`.

- [ ] **Step 9: Verify abnormal and normal exit semantics**

Record the exact installed Core PID, terminate only that process, and wait at most 90 seconds for a different PID plus `registered/running` status. Then use the tray or protected Lens exit action, wait 120 seconds, restart the service, and confirm Core remains suppressed. Start Core from the Start Menu and confirm `resume` clears suppression.

- [ ] **Step 10: Verify collection and upload after recovery**

Confirm current project count, capture state, queue depth, latest heartbeat, and a newly accepted server event. Verify no screenshot files exist and no credential plaintext appears in process command lines or logs.

---

### Task 9: Server Publication, Download Verification, And Documentation

**Files:**
- Modify: `docs/operations/implementation-log.md`
- Modify: `docs/operations/server-implementation-log.md`
- Modify server deployment environment: `/opt/aetheris/.env` through interactive SSH
- Create server artifact: `/opt/aetheris/public/downloads/AetherisSetup-0.4.8.exe`
- Create server checksum: `/opt/aetheris/public/downloads/AetherisSetup-0.4.8.exe.sha256`

**Interfaces:**
- Consumes: locally accepted `dist/AetherisSetup-0.4.8.exe` and checksum.
- Produces: `/downloads/client` serving the exact accepted installer.
- Preserves: current 0.4.7 server binaries, Admin Web, PostgreSQL, Qdrant, Ollama, and Model Gateway.

- [x] **Step 1: Run read-only server preflight using interactive password authentication**

Run: `ssh -p 22 root@192.168.78.138`

Enter the password only in the interactive SSH prompt. Inspect `/opt/aetheris`, active services, filesystem capacity, current download target, and existing backups. Do not overwrite any `/opt` path during preflight.

- [x] **Step 2: Create a timestamped rollback directory and upload to staging**

Run remotely: `install -d -m 0750 /opt/aetheris/staging/0.4.8 /opt/aetheris/backups/core-service-0.4.8-20260908`

Run locally: `scp -P 22 dist/AetherisSetup-0.4.8.exe dist/AetherisSetup-0.4.8.exe.sha256 root@192.168.78.138:/opt/aetheris/staging/0.4.8/`

Enter the SSH password interactively. Compare local and remote SHA-256 before promotion.

- [x] **Step 3: Promote the installer without changing unrelated services**

Copy the current public installer and checksum into the rollback directory, install the 0.4.8 files into `/opt/aetheris/public/downloads`, update only `CLIENT_DOWNLOAD_FILE` to `/opt/aetheris/public/downloads/AetherisSetup-0.4.8.exe`, and restart only `aetheris-server` if the environment is read at process start.

- [x] **Step 4: Verify HTTP and server health**

Run locally: `Invoke-WebRequest -UseBasicParsing http://192.168.78.138:8080/healthz`

Run locally: `Invoke-WebRequest -UseBasicParsing http://192.168.78.138:8080/downloads/client -OutFile build\downloaded-AetherisSetup-0.4.8.exe`

Run: `Get-FileHash build\downloaded-AetherisSetup-0.4.8.exe -Algorithm SHA256`

Expected: health is 200, Content-Disposition names 0.4.8, and HTTP/local/staging/public SHA-256 values are identical.

- [x] **Step 5: Verify disk and product services**

Confirm total `/opt/aetheris` usage remains under 100 GB. Verify Admin Web login page, device list, effectiveness dashboard, Gateway heartbeat/ingest, Model Gateway readiness, Ollama, PostgreSQL, and Qdrant remain healthy; do not run destructive data cleanup.

- [x] **Step 6: Record the final checkpoint**

Append the exact Core/service/Setup hashes, local SCM acceptance results, server backup path, HTTP verification, disk usage, and remaining user-performed reboot/login check to both operations logs. Do not record usernames, SID values, enrollment code, device/control token, server password, project paths, OCR text, or event payload.
