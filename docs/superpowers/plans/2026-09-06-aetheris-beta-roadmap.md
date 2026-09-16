# Aetheris Beta Roadmap

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver a usable Windows personal-developer Beta that turns authorized development signals into privacy-preserving, auditable work episodes and serves the four agreed outcomes: project memory, automatic development logs, explainable workflow insights, and reviewable AI-ready dataset exports.

**Architecture:** Use an enterprise-microservice-first contract while packaging the first release as a Windows Core client plus a private server. Core performs process consent, project attribution, in-memory OCR fallback, redaction, and bounded SQLite buffering. The server owns the authorized redacted copy, event ingestion, Forge processing, Nexus retrieval, Pulse aggregation, Lens review, and exports.

**Tech Stack:** Python 3.11+ for the current vertical slice, Windows 10/11 client, SQLite local queue, HTTP Gateway, server event store, Ollama by default, Dify as an optional orchestration adapter, JSON Schema, JSONL, Markdown, PowerShell deployment scripts.

**Spec:** `docs/superpowers/specs/2026-09-06-aetheris-architecture-design.md`

## Global Constraints

- Only authorized, project-attributed, device-redacted events may leave Core.
- Screenshots and video frames are memory-only and never written to disk.
- High-risk originals are isolated locally for 24 hours by default and never uploaded in Beta.
- Local SQLite default limit is 512 MB; recent cache retention is 7 days.
- Server deployment root is `/opt/aetheris` on `192.168.78.138`.
- Server data and service files must stay below a hard 100 GB budget.
- The root password previously shown in chat must never appear in source, scripts, configuration, logs, or test fixtures.
- Process discovery precedes content capture; system, IM, printing, security, and password-manager processes are excluded by default.
- Browser collection is plugin-free, allowlist-only, based on visible window sampling/OCR and scroll stitching.
- AI IDE integrations prefer read-only structured local session files/databases; OCR is fallback only.
- Ollama is the default model runtime; Dify is optional and never owns canonical Aetheris data.
- Pulse provides explainable workflow insights and never employee rankings or opaque productivity scores.

## Current Baseline

The repository already contains a partial vertical slice:

- Canonical `AetherisEvent` serialization and hashing.
- Process classification and project-root authorization primitives.
- Deterministic redaction rules.
- SQLite queue with claim, acknowledgement, release, and rejection states.
- Gateway ingestion with bearer authentication and event idempotency.
- Server event storage, summaries, device registration, and export endpoints.
- Git and terminal adapters.
- Windows bundle, installer scripts, tray configuration, and local review page.

Current baseline: the P0/P1/P2/P3/P4/P5/P6 work completed so far is covered by 44 passing standard-library tests. The remaining roadmap items are tracked in `docs/operations/implementation-log.md` and must not be represented as complete until their adapters and acceptance tests exist.

## Phase P0: Restore A Green Baseline

### Task P0.1: Fix legacy Windows path migration

**Files:** `src/aetheris/tray.py`, `tests/test_tray.py`

- [ ] Normalize legacy single-backslash Windows paths without converting `\t`, `\n`, or other sequences into control characters.
- [ ] Preserve existing absolute-path validation and project-root checks.
- [ ] Add coverage for drive paths, UNC paths, and temporary directories.
- [ ] Run `python -m unittest tests.test_tray -v`.

### Task P0.2: Align admin page contracts

**Files:** `src/aetheris/gateway.py`, `src/aetheris/server.py`, `tests/test_admin_auth.py`, `tests/test_server_workspace.py`

- [ ] Make first-login HTML expose the stable `Admin login` contract expected by tests and users.
- [ ] Make authenticated device management expose the stable `Aetheris Device Management` contract.
- [ ] Preserve the forced first-password-change flow.
- [ ] Add tests for failed login, forced change, logout, and authenticated state access.
- [ ] Run the two focused test modules.

### Task P0.3: Verify the clean baseline

- [ ] Run `python -m unittest discover -s tests -v`.
- [ ] Run repository secret-literal scanning.
- [ ] Record the final pass count in the implementation log.

## Phase P1: Reliable Client-to-Server Delivery

### Task P1.1: Versioned event schema

**Files:** `src/aetheris/events.py`, `schemas/aetheris-event-v1.schema.json`, `tests/test_events.py`

- [ ] Publish the required envelope fields and permitted event types.
- [ ] Validate field types, timestamps, grants, crypto mode, and content hash.
- [ ] Return field-level rejection reasons from Gateway.
- [ ] Add forward-compatible handling for optional fields.

### Task P1.2: Bounded SQLite queue

**Files:** `src/aetheris/queue.py`, `tests/test_redaction_and_queue.py`

- [ ] Enable WAL mode and transactional claim/ack/release behavior.
- [ ] Track byte usage, oldest pending event, and queue watermarks.
- [ ] Enforce 512 MB default limit without deleting unacknowledged rows.
- [ ] Emit a local diagnostic event at 80% and pause nonessential capture at 95%.

### Task P1.3: Retry, replay, and deletion semantics

**Files:** `src/aetheris/gateway.py`, `src/aetheris/server.py`, `src/aetheris/queue.py`, tests

- [ ] Add exponential backoff and bounded retries.
- [ ] Preserve event order within a batch.
- [ ] Keep malformed events locally inspectable without retrying forever.
- [ ] Add deletion tombstones that prevent old clients from resurrecting events.
- [ ] Verify duplicate upload remains idempotent.

## Phase P2: Process Discovery and Consent

### Task P2.1: Process identity and classification

**Files:** `src/aetheris/processes.py`, `src/aetheris/consent.py`, tests

- [ ] Read minimum process metadata only before consent.
- [ ] Verify executable path, publisher, signature status, version, and binary hash.
- [ ] Exclude Windows system, IM, printing, security, credentials, and Aetheris processes.
- [ ] Classify known development tools, unknown tools, and child processes.

### Task P2.2: Consent state machine

- [ ] Support session, project-persistent, global, denied, and always-ignore grants.
- [ ] Revoke grants when publisher/path/hash changes.
- [ ] Persist consent decisions as auditable events.
- [ ] Expose pending unknown processes to Lens and the tray.

### Task P2.3: Project attribution

- [ ] Resolve Git and supported project roots.
- [ ] Require explicit authorized roots.
- [ ] Route ambiguous events to `pending_classification`.
- [ ] Test parent-process inheritance with project-bound child processes.

## Phase P3: Capture Adapters

Implement each adapter as a read-only, version-aware module with fixtures, redaction coverage, source provenance, and failure fallback.

- [ ] Git commits, branches, diff statistics, and validation results.
- [ ] PowerShell and Windows Terminal commands/results.
- [ ] VS Code workspace activity.
- [ ] VS Code Chat/GitHub Copilot sessions.
- [ ] Codex local sessions or exports.
- [ ] Claude Code local sessions or exports.
- [ ] Cursor local SQLite/Markdown chat history.
- [ ] Visual Studio activity.
- [ ] Browser visible-window OCR for allowlisted domains only.
- [ ] Generic unknown-tool screenshot/OCR fallback.

Browser requirements:

- [ ] No browser extension, CDP, or special browser profile.
- [ ] Sample only visible foreground windows and user-scrolled content.
- [ ] Ignore non-allowlisted domains completely.
- [ ] Keep screenshots in memory only.
- [ ] Redact before any persistence or upload.

## Phase P4: Forge and Work Episodes

### Task P4.1: Normalize and correlate events

- [ ] Deduplicate by content hash and source identity.
- [ ] Correlate events by project, session, and correlation ID.
- [ ] Preserve immutable facts; corrections create superseding events.
- [ ] Track provenance and processing grants.

### Task P4.2: Generate `WorkEpisode`

- [ ] Group objective, context, AI interaction, actions, decisions, validation, and outcome.
- [ ] Mark uncertain conclusions as `needs_review`.
- [ ] Store evidence references for every derived fact.
- [ ] Recompute derived episodes after correction or deletion.

## Phase P5: Four Beta Outcomes

### Task P5.1: Nexus project memory assistant

- [ ] Implement retrieval over Work Episodes and source events.
- [ ] Return citations for every factual answer.
- [ ] Return explicit uncertainty when evidence is insufficient.
- [ ] Integrate Ollama first and Dify through an adapter.
- [ ] Remove deleted/denied events from retrieval indexes.

### Task P5.2: Automatic development log

- [ ] Generate daily completed work, problems, decisions, verification, and next actions.
- [ ] Preserve event references for every line.
- [ ] Support user correction and regeneration.
- [ ] Export Markdown and JSON.

### Task P5.3: Explainable workflow insights

- [ ] Classify coding, debugging, reading, AI collaboration, testing, waiting, and switching.
- [ ] Detect repeated failures, rework, and blocking intervals.
- [ ] Provide drill-down evidence.
- [ ] Avoid keyboard counts, mouse counts, online-time scoring, and employee ranking.

### Task P5.4: AI-ready dataset exports

- [ ] Generate Work Episode JSONL.
- [ ] Generate one Markdown document per episode.
- [ ] Generate OpenAI-compatible `messages.jsonl`.
- [ ] Generate Agent trajectory JSONL.
- [ ] Include manifest, schemas, provenance, redaction report, rights manifest, hashes, and data sheet.
- [ ] Block export when `dataset_export` is not granted.

## Phase P6: Lens and User Controls

- [ ] Show active processes, grants, projects, pending classification, isolation, and sync health.
- [ ] Allow pause/resume by source and collector capability.
- [ ] Allow correction, reassignment, deletion, and export review.
- [ ] Show provenance and redaction decisions without exposing quarantined originals.
- [ ] Show local SQLite usage and server synchronization state.

## Phase P7: Private Server Deployment

**Target:** `192.168.78.138:/opt/aetheris`

- [ ] Perform read-only OS, CPU, memory, disk, network, and `/opt` inspection.
- [ ] Create a non-root `aetheris` service account.
- [ ] Use SSH keys; never place the previously exposed root password in files or commands.
- [ ] Deploy Gateway, Forge, event store, object storage, vector index, Lens, Nexus, and Pulse.
- [ ] Enforce a 100 GB budget with 80 GB warning, 90 GB nonessential-write stop, and 95 GB intake pause.
- [ ] Rotate logs and expire backups.
- [ ] Verify restart, rollback, health checks, and offline client recovery.

## Phase P8: Security and Beta Acceptance

- [ ] Seed test data with fake API keys, passwords, cookies, emails, phone numbers, and local paths.
- [ ] Verify none reach server storage or exports.
- [ ] Verify screenshots never reach disk.
- [ ] Verify denied processes and unauthorized projects produce no uploadable content.
- [ ] Verify allowlist-only browser capture.
- [ ] Verify cascade deletion of documents, vectors, summaries, logs, and insights.
- [ ] Verify deleted events cannot be resurrected by an offline client.
- [ ] Run the 10-workday real-project Beta acceptance protocol.
- [ ] Document known adapter limitations and unsupported applications.

## Execution Order

```text
P0 baseline repair
  -> P1 reliable delivery
  -> P2 process discovery and consent
  -> P3 capture adapters
  -> P4 Forge and Work Episodes
  -> P5 Nexus, logs, Pulse, dataset exports
  -> P6 Lens
  -> P7 private server deployment
  -> P8 security and Beta acceptance
```

## Explicitly Out Of Scope For This Beta

- Employee rankings or opaque productivity scores.
- Browser extensions and continuous video recording.
- Automatic public data trading or marketplace listing.
- Full IDE/platform coverage.
- Model fine-tuning platform.
- Multi-region high availability.
- Automatic capture of web content the user never viewed.
