# Aetheris Architecture Design

**Date:** 2026-09-06  
**Status:** Confirmed for Beta vertical slice  
**Scope:** Windows 10/11 personal developer Beta with a private-server deployment path

## 1. Product and delivery goals

Aetheris turns developer work signals into inspectable, privacy-preserving work episodes. The first release is a personal developer Beta and must run on Windows 10/11 with VS Code, Visual Studio, Git CLI, PowerShell/Windows Terminal, Chrome/Edge, and the first AI tools: VS Code Chat/GitHub Copilot, Codex, Claude Code, and Cursor.

The product modules keep stable boundaries:

- **Core:** Windows process discovery, adapter execution, project identification, consent enforcement, capture scheduling, and local delivery.
- **Lens:** source visibility, correction, deletion, consent review, and provenance inspection.
- **Forge:** normalization, redaction policy, event validation, quality checks, and derived-event production.
- **Nexus:** retrieval, citations, exports, and model orchestration (Ollama by default, Dify optional).
- **Pulse:** explainable individual and team workflow insights. It never produces opaque employee scores or rankings.

The first vertical slice is intentionally narrower: Core discovers a process and project, redacts a sample payload, queues an immutable `AetherisEvent` in SQLite, sends it through Gateway, and stores it in the server event database.

## 2. Architecture shape

The target architecture is enterprise-microservice-first, while Beta runs as a local Windows collector plus a private server. Contracts are shared even when components run in one process. The first implementation uses Python 3.11 standard-library services to keep the Windows installer and private-server bootstrap small; each boundary is represented by a module that can later become an independently deployed service.

```text
Windows Core
  process discovery -> consent gate -> project resolver -> redactor
  -> bounded SQLite queue (encrypted sensitive fields, no screenshots)
  -> HTTPS Gateway (outbound only)
  -> Forge validation/normalization
  -> server event store
  -> future Lens/Nexus/Pulse consumers
```

### Deployment modes

1. **Personal Beta:** Core and local SQLite run on the developer device. Gateway and event store run on the configured private server. The client sends only authorized, project-attributed, device-redacted events.
2. **Enterprise evolution:** Gateway, Forge, Audit, event storage, object storage, vector index, and Model Gateway are independently deployable behind an OIDC/mTLS boundary. The event contract and privacy rules remain unchanged.

## 3. Trust and data boundaries

- Screenshots are memory-only and are never written to disk.
- Normal text is redacted before persistence. High-risk original text may enter an encrypted isolation area on the device for 24 hours by default, but is never uploaded in the Beta vertical slice.
- The process classifier runs before capture. System, IM, printing, and security processes are excluded by default. Unknown processes remain in a pending-confirmation state.
- Browser capture is limited to an explicit domain allowlist. Non-allowlisted windows are ignored. Browser acquisition is window sampling plus OCR/scroll stitching in a later adapter; no browser plugin is required.
- IDE AI adapters prefer read-only parsing of local session files/databases. Screenshot/OCR is a fallback only when no structured source is available.
- A process must have an active user grant and a project root authorization before its content can be queued.
- The server receives device-redacted content only. A future dual-vault mode can add user-selected end-to-end encrypted content without changing the event semantics.
- No credential, including any root password that may have appeared in chat, is stored in source, scripts, config, tests, or logs. Server administration uses SSH keys and a non-root service account.

## 4. Canonical data model

### 4.1 `AetherisEvent`

Events are immutable facts. Corrections create a new event with `supersedes_event_id`; deletion creates an auditable tombstone. Derived summaries and insights always reference their input event IDs.

Required envelope fields:

```json
{
  "event_id": "uuid",
  "schema_version": 1,
  "event_type": "process.observed",
  "tenant_id": "local-default",
  "subject_id": "local-user",
  "device_id": "device-...",
  "project_id": "project-...",
  "session_id": "session-...",
  "correlation_id": "correlation-...",
  "source": "core.process",
  "source_version": "0.1.0",
  "occurred_at": "RFC3339 UTC",
  "ingested_at": "RFC3339 UTC",
  "payload": {},
  "content_refs": [],
  "content_hash": "sha256",
  "provenance": {},
  "redaction_report": {},
  "consent_policy_version": "1",
  "processing_grants": ["server_ingest"],
  "crypto_mode": "device-redacted",
  "key_version": null
}
```

The Beta slice requires `event_id`, `schema_version`, `event_type`, tenant/device/project identity, source/version, occurrence time, payload, content hash, redaction report, and processing grants. Optional fields must remain forward-compatible.

### 4.2 External `WorkEpisode`

`WorkEpisode` is a consumer-facing projection, not a replacement for the internal fact model. It groups related events by project/session and exposes explainable evidence references:

```json
{
  "episode_id": "episode-...",
  "project_id": "project-...",
  "started_at": "RFC3339 UTC",
  "ended_at": "RFC3339 UTC",
  "event_ids": ["..."],
  "evidence": [{"event_id": "...", "reason": "same session"}],
  "summary": null
}
```

## 5. Beta component contracts

### Core

- `ProcessDiscoverer.discover() -> list[ProcessObservation]`
- `ConsentRegistry.classify(observation) -> ProcessDecision`
- `ProjectResolver.resolve(path, authorized_roots) -> ProjectRef | None`
- `Redactor.redact(value) -> (redacted_value, RedactionReport)`
- `LocalQueue.enqueue(event)`, `LocalQueue.claim_batch(limit)`, `LocalQueue.ack(event_ids)`

The default process source uses `tasklist` on Windows and a portable fallback for development. Project recognition walks upward for `.git`, `pyproject.toml`, `package.json`, `go.mod`, or `.sln`; only explicitly authorized roots may produce a project ID.

The first post-slice adapter is the Git CLI adapter. It invokes Git read-only, emits the latest commit's hash, subject, author name, hashed author email, and authored time, plus aggregate `git diff --numstat` counts. Patch bodies are deliberately outside the first adapter contract.

### Gateway

The client sends a JSON object with an `events` array to `POST /v1/events`. The request is authenticated by a device token in the `Authorization: Bearer` header in the Beta harness. The server returns `202` with per-event accepted/rejected results. Duplicate event IDs are idempotent and return `duplicate`.

`GET /healthz` returns a small JSON readiness response and does not expose secrets or event content.

### Server event store

The private server persists the canonical envelope in SQLite for the Beta slice. A unique index on `event_id` enforces idempotency. The schema keeps the complete redacted event JSON, hashes, timestamps, and event type in queryable columns. The service binds to the private interface only when deployed remotely; local tests bind to loopback.

## 6. Redaction policy

The first rules are deterministic and auditable:

- API keys, bearer tokens, private keys, passwords, and connection-string credentials become typed placeholders such as `[REDACTED:token]`.
- Email addresses and absolute home-directory paths are replaced while preserving enough shape for debugging.
- Hashes are computed from the redacted canonical JSON, never from high-risk originals.
- `RedactionReport` records rule IDs and replacement counts, not the original values.

The policy is deliberately conservative. If a value cannot be classified confidently, it is marked `needs_review` and is not uploaded by the Beta client.

## 7. Local storage and retention

- SQLite default maximum: **512 MB**.
- Recent cache retention: **7 days**.
- High-risk isolation retention: **24 hours**.
- Successful server acknowledgements permit LRU deletion of old正文/cache rows; unacknowledged queue rows are retained first.
- Near the quota, Core pauses new capture and emits a local diagnostic event rather than silently dropping data.
- SQLite never stores screenshots. Application-layer encryption uses Windows DPAPI in the full client; the vertical slice stores only redacted JSON and keeps the encryption interface explicit for later replacement.

## 8. Server capacity and operations

The configured server is `192.168.78.138`, deployment root `/opt/aetheris`, with a hard disk budget of **100 GB**. Before any deployment command, the directory is inspected read-only; existing `/opt` content is never overwritten by this project. The initial service account is non-root, SSH uses keys, logs exclude payloads and credentials, and retention/rotation jobs must enforce the disk budget.

The Windows Core Beta executable is served from a fixed public download directory. The build produces a versioned PyInstaller tray executable and checksum; no IExpress/WinCab wrapper is used. Gateway exposes `/` and `/downloads/` for installation discovery, allows only the versioned executable and checksum, and rejects traversal or arbitrary file reads. The executable's first-run UI discovers or selects a Git root, writes configuration, and performs a Gateway health check before entering the tray.

## 9. Export and model boundaries

Exports are generated from canonical events and their provenance: JSONL, Markdown, OpenAI messages JSONL, and Agent trajectory. Ollama is the default runtime. Dify is an optional orchestration layer and never owns Aetheris storage, indexing, retrieval, or citations.

The Beta Gateway exposes authenticated management APIs: `/v1/devices/register` and `/v1/devices` for client registration and inventory, `/v1/events` for bounded event queries, `/v1/summary` for project/type organization, and `/v1/export/events.{jsonl,markdown,openai_messages,agent_trajectory}` for structured exports. `/admin` is a token-entry browser view over these APIs. Personal event review is local-only through the Core tray's loopback Lens.

## 10. Observability and failure behavior

- Every event has a stable ID and content hash.
- Client upload retries are bounded and preserve queue order within a batch.
- A malformed event is rejected with a field-level reason and remains locally inspectable; it is not retried forever.
- A temporary Gateway failure leaves the event queued.
- Logs contain event IDs, counts, status, and latency only; never payloads, tokens, passwords, or raw OCR.
- All acceptance tests use a temporary SQLite database and loopback HTTP server.

## 11. Acceptance criteria for the first vertical slice

1. A clean checkout starts the Gateway and server store with one documented command.
2. A Core capture fixture representing an allowed process and authorized project creates a redacted `AetherisEvent`.
3. The event survives a local SQLite enqueue/claim cycle.
4. Gateway accepts it exactly once; a replay is reported as a duplicate without a second row.
5. The server database contains the redacted event and no original secret.
6. A denied process, unknown process, unapproved project root, or failed redaction never reaches the server.
7. The end-to-end test runs without external services, network access, or credentials.
