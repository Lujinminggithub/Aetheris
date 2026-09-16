# Aetheris Vertical Slice Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build and verify the first runnable Aetheris path from Windows Core process/project discovery through redaction, bounded SQLite queue, Gateway, and server event storage.

**Architecture:** Use small Python 3.11 modules with standard-library HTTP and SQLite APIs. Core emits versioned `AetherisEvent` envelopes, the local queue provides durable upload state, Gateway validates/authenticates/idempotently stores redacted events, and a loopback end-to-end test proves the full path without external services.

**Tech Stack:** Python 3.11+, `sqlite3`, `http.server`, `argparse`, `hashlib`, `uuid`, standard-library `unittest`, PowerShell launch scripts.

**Spec:** `docs/superpowers/specs/2026-09-06-aetheris-architecture-design.md`

## Global Constraints

- Windows 10/11 is the first client target; development fixtures must run on any OS.
- Screenshots never touch disk; the vertical slice contains no screenshot persistence API.
- Only device-redacted, project-attributed, authorized events may leave Core.
- High-risk original text is never uploaded and never appears in tests, logs, or fixtures.
- Default local queue limit is 512 MB; recent cache is 7 days; isolation is 24 hours.
- Gateway accepts `POST /v1/events` and returns per-event idempotent results.
- Server deployment root is `/opt/aetheris`; server inspection is read-only until separately approved.
- No root password or other credential may be written to repository files, commands, or logs.

### Task 1: Bootstrap the Python package and test harness

**Files:**
- Create: `pyproject.toml`
- Create: `src/aetheris/__init__.py`
- Create: `src/aetheris/version.py`
- Create: `tests/test_bootstrap.py`
- Create: `scripts/run_gateway.ps1`

**Interfaces:**
- Produces importable package `aetheris` and `aetheris.__version__`.
- Produces a documented PowerShell entry point used by later tasks.

- [ ] **Step 1: Write the failing test**

```python
def test_package_exposes_version():
    import aetheris
    assert aetheris.__version__ == "0.1.0"
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m unittest tests.test_bootstrap -v`
Expected: FAIL with `ModuleNotFoundError: No module named 'aetheris'`.

- [ ] **Step 3: Write minimal implementation**

Create `src/aetheris/version.py` with `__version__ = "0.1.0"`, export it from `src/aetheris/__init__.py`, and configure `pyproject.toml` with a `src` layout. Keep `scripts/run_gateway.ps1` as a thin launcher that calls `python -m aetheris.gateway`.

- [ ] **Step 4: Run test to verify it passes**

Run: `python -m unittest tests.test_bootstrap -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add pyproject.toml src tests scripts/run_gateway.ps1
git commit -m "chore: bootstrap aetheris python package"
```

If Git metadata is still absent, record the exact limitation in the handoff instead of creating unrelated repository metadata.

### Task 2: Define event contracts and canonical serialization

**Files:**
- Create: `src/aetheris/events.py`
- Create: `tests/test_events.py`

**Interfaces:**
- `AetherisEvent.create(event_type, *, tenant_id, subject_id, device_id, project_id, session_id, source, source_version, payload, redaction_report, processing_grants) -> AetherisEvent`
- `AetherisEvent.to_dict() -> dict`
- `AetherisEvent.to_json() -> str`
- `AetherisEvent.content_hash -> str`
- `AetherisEvent.from_dict(data) -> AetherisEvent`

- [ ] **Step 1: Write the failing test**

```python
def test_event_serialization_is_deterministic_and_contains_redaction_metadata():
    event = AetherisEvent.create(
        "process.observed", tenant_id="local-default", subject_id="local-user",
        device_id="device-test", project_id="project-a",
        session_id="session-a", source="core.process", source_version="0.1.0",
        payload={"name": "code.exe"}, redaction_report={"rules": []},
        processing_grants=["server_ingest"],
    )
    data = json.loads(event.to_json())
    assert data["schema_version"] == 1
    assert data["content_hash"] == event.content_hash
    assert data["redaction_report"] == {"rules": []}
    assert AetherisEvent.from_dict(data).to_json() == event.to_json()
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m unittest tests.test_events -v`
Expected: FAIL because `AetherisEvent` is not defined.

- [ ] **Step 3: Write minimal implementation**

Implement a frozen dataclass, UTC RFC3339 timestamps, UUID event IDs, canonical JSON with sorted keys and compact separators, and SHA-256 over the envelope without the `content_hash` field. Validate required non-empty identity fields and reject a mismatched supplied hash.

- [ ] **Step 4: Run test to verify it passes**

Run: `python -m unittest tests.test_events -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add src/aetheris/events.py tests/test_events.py
git commit -m "feat: add canonical aetheris event contract"
```

### Task 3: Implement process classification and project authorization

**Files:**
- Create: `src/aetheris/core.py`
- Create: `tests/test_core_capture.py`

**Interfaces:**
- `ProcessObservation(pid: int, name: str, executable: str | None, command_line: str | None)`
- `ProcessDecision(status: Literal["allowed", "excluded", "pending"], reason: str)`
- `ConsentRegistry.classify(observation) -> ProcessDecision`
- `ProjectResolver.resolve(path, authorized_roots) -> ProjectRef | None`
- `capture_observation(observation, *, project_path, consent_registry, authorized_roots) -> AetherisEvent | None`

- [ ] **Step 1: Write the failing test**

```python
def test_allowed_process_and_authorized_git_root_create_event(tmp_path):
    (tmp_path / ".git").mkdir()
    registry = ConsentRegistry(allowed_names={"code.exe"})
    observation = ProcessObservation(42, "Code.exe", str(tmp_path / "Code.exe"), None)
    event = capture_observation(observation, project_path=tmp_path,
                                consent_registry=registry,
                                authorized_roots=[tmp_path])
    assert event.event_type == "process.observed"
    assert event.project_id.startswith("project-")

def test_unknown_process_and_unapproved_root_do_not_create_event(tmp_path):
    observation = ProcessObservation(7, "mystery.exe", None, None)
    registry = ConsentRegistry(allowed_names={"code.exe"})
    assert capture_observation(observation, project_path=tmp_path,
                               consent_registry=registry,
                               authorized_roots=[]) is None
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m unittest tests.test_core_capture -v`
Expected: FAIL because the Core interfaces do not exist.

- [ ] **Step 3: Write minimal implementation**

Normalize process names case-insensitively, exclude system/IM/printing/security patterns, return `pending` for unknown names, identify project roots by walking upward for `.git`, `pyproject.toml`, `package.json`, `go.mod`, or `.sln`, and require the resolved path to be under an authorized root. Generate a deterministic project ID from the normalized root path and include only non-sensitive process metadata in the payload.

- [ ] **Step 4: Run test to verify it passes**

Run: `python -m unittest tests.test_core_capture -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add src/aetheris/core.py tests/test_core_capture.py
git commit -m "feat: classify processes and authorize projects"
```

### Task 4: Add deterministic redaction and bounded SQLite queue

**Files:**
- Create: `src/aetheris/redaction.py`
- Create: `src/aetheris/queue.py`
- Create: `tests/helpers.py`
- Create: `tests/test_redaction_and_queue.py`

**Interfaces:**
- `Redactor.redact(value) -> tuple[object, dict]`
- `LocalQueue(path, max_bytes=512 * 1024 * 1024)`
- `LocalQueue.enqueue(event) -> None`
- `LocalQueue.claim_batch(limit) -> list[AetherisEvent]`
- `LocalQueue.ack(event_ids) -> None`
- `LocalQueue.release(event_ids) -> None`
- `LocalQueue.reject(event_ids, reason) -> None`
- `make_sample_event() -> AetherisEvent` returns a valid deterministic event with a harmless process payload.

- [ ] **Step 1: Write the failing test**

```python
def test_redaction_removes_secret_before_event_hashing():
    value, report = Redactor().redact({"token": "Bearer abc123", "note": "hello"})
    assert value["token"] == "[REDACTED:token]"
    assert report["replacement_count"] == 1
    assert "abc123" not in json.dumps(value)

def test_queue_round_trip_and_ack(self):
    sample_event = make_sample_event()
    queue = LocalQueue(tmp_path / "queue.db", max_bytes=1024 * 1024)
    queue.enqueue(sample_event)
    claimed = queue.claim_batch(10)
    assert [item.event_id for item in claimed] == [sample_event.event_id]
    queue.ack([sample_event.event_id])
    assert queue.claim_batch(10) == []
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m unittest tests.test_redaction_and_queue -v`
Expected: FAIL because the redactor and queue are not implemented.

- [ ] **Step 3: Write minimal implementation**

Create `tests/helpers.py` with `make_sample_event()` calling `AetherisEvent.create` using fixed IDs and `{"name": "code.exe"}`. Apply regex rules for bearer/API tokens, private-key blocks, password assignments, emails, and user-home paths; return rule IDs and counts only. Store canonical event JSON in SQLite with `status` (`queued`/`inflight`), `attempts`, and timestamps. Enforce the byte limit before insertion and keep an atomic claim transaction so concurrent upload loops cannot claim the same event.

- [ ] **Step 4: Run test to verify it passes**

Run: `python -m unittest tests.test_redaction_and_queue -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add src/aetheris/redaction.py src/aetheris/queue.py tests/test_redaction_and_queue.py
git commit -m "feat: redact payloads and persist bounded upload queue"
```

### Task 5: Implement Gateway and server event store

**Files:**
- Create: `src/aetheris/server.py`
- Create: `src/aetheris/gateway.py`
- Create: `tests/test_gateway.py`

**Interfaces:**
- `EventStore(path).insert_if_new(event) -> Literal["accepted", "duplicate"]`
- `EventStore.count() -> int`
- `create_gateway_server(store, token) -> ThreadingHTTPServer`
- `GatewayClient(base_url, token).send(events) -> list[dict]`

- [ ] **Step 1: Write the failing test**

```python
def test_gateway_accepts_event_once_and_rejects_bad_token(self):
    sample_event = make_sample_event()
    store = EventStore(tmp_path / "server.db")
    server = create_gateway_server(store, token="test-token")
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        client = GatewayClient(f"http://127.0.0.1:{server.server_port}", "test-token")
        assert client.send([sample_event])[0]["status"] == "accepted"
        assert client.send([sample_event])[0]["status"] == "duplicate"
        assert store.count() == 1
    finally:
        server.shutdown()
        thread.join(timeout=2)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m unittest tests.test_gateway -v`
Expected: FAIL because Gateway and EventStore do not exist.

- [ ] **Step 3: Write minimal implementation**

Use `http.server.ThreadingHTTPServer`. Refuse to start without a non-empty device token. `GET /healthz` returns `{"status":"ok"}`. `POST /v1/events` requires the exact bearer token, accepts a JSON object with an `events` array, validates every event through `AetherisEvent.from_dict`, inserts atomically by `event_id`, and returns `202` with per-event statuses. Never log request bodies.

- [ ] **Step 4: Run test to verify it passes**

Run: `python -m unittest tests.test_gateway -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add src/aetheris/server.py src/aetheris/gateway.py tests/test_gateway.py
git commit -m "feat: add idempotent gateway and event store"
```

### Task 6: Prove the Core-to-server vertical slice

**Files:**
- Create: `src/aetheris/cli.py`
- Create: `tests/test_vertical_slice.py`
- Modify: `scripts/run_gateway.ps1`
- Modify: `README.md`

**Interfaces:**
- `run_capture_and_upload(observation, project_path, authorized_roots, queue, gateway_client) -> dict`
- CLI commands: `python -m aetheris.cli demo --server-url ... --token ... --queue ...`

- [ ] **Step 1: Write the failing test**

```python
def test_core_to_gateway_to_server_contains_only_redacted_content(tmp_path):
    project = tmp_path / "repo"
    (project / ".git").mkdir(parents=True)
    queue = LocalQueue(tmp_path / "client.db")
    store = EventStore(tmp_path / "server.db")
    server = create_gateway_server(store, token="test-token")
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        result = run_capture_and_upload(
            ProcessObservation(1, "Code.exe", None, None), project,
            [project], queue,
            GatewayClient(f"http://127.0.0.1:{server.server_port}", "test-token"),
        )
        assert result["accepted"] == 1
        row = store.fetch_all()[0]
        assert "Bearer abc123" not in row["event_json"]
        assert "[REDACTED:token]" in row["event_json"]
    finally:
        server.shutdown()
        thread.join(timeout=2)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m unittest tests.test_vertical_slice -v`
Expected: FAIL because the orchestration function and CLI do not exist.

- [ ] **Step 3: Write minimal implementation**

Compose Core capture, redaction, event creation, queue enqueue/claim, Gateway upload, and acknowledgement. Use a fixed safe fixture payload in `demo` so no machine process command line or secret is collected during the demonstration. Document startup, tests, privacy guarantees, and the absence of credentials in `README.md`.

- [ ] **Step 4: Run the focused and full test suites**

Run: `python -m unittest tests.test_vertical_slice -v` and then `python -m unittest discover -s tests -v`.
Expected: both commands exit 0 with no failures.

- [ ] **Step 5: Commit**

```powershell
git add src/aetheris/cli.py tests/test_vertical_slice.py scripts/run_gateway.ps1 README.md
git commit -m "feat: verify core to server vertical slice"
```

### Task 7: Perform verification and server read-only preflight

**Files:**
- Create: `docs/operations/server-read-only-preflight.md`
- Create: `tests/test_no_sensitive_literals.py`

- [ ] **Step 1: Write the failing repository safety test**

```python
def test_repository_contains_no_forbidden_secret_literals():
    forbidden = ["BEGIN " + "RSA PRIVATE KEY", "Authorization: " + "Bearer abc123"]
    text = "\\n".join(path.read_text(errors="ignore") for path in Path(".").rglob("*") if path.is_file())
    assert all(value not in text for value in forbidden)
```

- [ ] **Step 2: Run it and fix only if it fails**

Run: `python -m unittest tests.test_no_sensitive_literals -v`.

- [ ] **Step 3: Run full verification**

Run: `python -m unittest discover -s tests -v` and `python -m compileall src tests`.
Expected: all tests pass and compilation exits 0.

- [ ] **Step 4: Read-only server checks**

Use SSH key authentication only. Run read-only commands against `192.168.78.138` such as `uname -a`, `df -h`, `id`, `ls -ld /opt /opt/aetheris`, and `find /opt/aetheris -maxdepth 2 -type f -printf '%p %s\\n'`. Do not create, delete, chmod, chown, or overwrite anything. If key-based access is unavailable, record the exact connection limitation without attempting password authentication.

- [ ] **Step 5: Record results**

Write command names, timestamps, exit codes, and non-sensitive summaries to `docs/operations/server-read-only-preflight.md`; omit IP credentials, private key paths, payloads, and command output that could expose secrets.
