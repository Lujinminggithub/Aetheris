from __future__ import annotations

import argparse
import ctypes
import hashlib
import json
import os
import re
import secrets
import socket
import sys
import threading
import time
import webbrowser
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timezone
from urllib.error import HTTPError
from dataclasses import dataclass, field, replace
from pathlib import Path

from .cli import run_capture_and_upload
from .core import ConsentRegistry, ProcessDiscoverer, ProcessObservation, ProjectResolver
from .gateway import GatewayClient, _read_token_file
from .queue import LocalQueue
from .local_view import create_local_view_server
from .adapters.git import GitAdapter
from .adapters.svn import SvnAdapter
from .adapters.terminal import TerminalAdapter
from .adapters.vscode import VSCodeAdapter
from .adapters.ai_sessions import AISessionAdapter
from .adapters.copilot import CopilotSessionAdapter
from .adapters.cursor import CursorSessionAdapter
from .adapters.visual_studio import VisualStudioAdapter
from .adapters.visual_studio_events import VisualStudioEventAdapter
from .adapters.window import collect_visible_window
from .adapters.ocr import NodeTesseractEngine, OcrCapture
from .dlp import DlpMatcher
from .credentials import CredentialStore, ProjectIdentityKeyStore
from .projects import ProjectSource
from .single_instance import SingleInstance, existing_status_url
from .status import CoreStatus, HeartbeatBackoff
from .version import __version__
from .project_registry import ProjectRegistry
from .command_privacy import load_or_create_command_privacy
from .project_attribution import attribute_ai_project
from .project_identity_sync import ProjectIdentityKeySync
from .project_sync import ProjectSync
from .process_consent import ProcessConsentCoordinator, StableProcessIdentity, can_capture
from .identity import default_user_identifier
from .adapter_health import AdapterHealthRegistry
from .processes import NativeProcessIdentityProvider
from .local_episodes import build_local_episodes
from .lifecycle import LifecycleLog
from .service_ipc import ServiceClient, ServiceUnavailable
from .vscode_bridge import VSCodeBridgeProcessor, VSCodeBridgeServer
from .vscode_installation import VSCodeInstallation
from .command_correlation import CommandOriginCorrelator
from .process_lineage import NativeProcessLineageProvider
from .application_capture_policy import ApplicationCapturePolicy, CaptureCandidate
from .adapters.application_window import ApplicationWindowInspector, ClientAreaCapture
from .adapters.application_ocr import ApplicationCaptureCandidate, ApplicationOcrAdapter

DEFAULT_GATEWAY_URL = "http://192.168.78.138:8080"


def default_config_path(base_dir: str | Path | None = None) -> Path:
    if base_dir:
        root = Path(base_dir)
    elif getattr(sys, "frozen", False):
        root = Path(sys.executable).resolve().parent
    else:
        root = Path.cwd()
    return root / "config" / "aetheris.json"


def default_device_id() -> str:
    return "device-" + hashlib.sha256(socket.gethostname().encode("utf-8")).hexdigest()[:16]


def build_config_data(*, gateway_url: str, token_file: Path, project_root: Path, queue: Path) -> dict:
    return {
        "gateway_url": gateway_url.rstrip("/"),
        "token_file": str(token_file.expanduser().resolve()),
        "project_root": str(project_root.expanduser().resolve()),
        "queue": str(queue.expanduser().resolve()),
        "device_id": default_device_id(),
        "executable": "AetherisCore.exe",
        "interval_seconds": 15,
        "local_view_port": 15473,
        "vscode_state_path": None,
        "ai_session_roots": [{"tool": tool, "path": str(path)} for tool, path in default_ai_session_roots()],
        "browser_allowlist": [],
        "browser_policy_revision": 0,
        "application_capture_enabled": True,
        "application_capture_policy_revision": 0,
    }


def default_ai_session_roots() -> list[tuple[str, Path]]:
    roots: list[tuple[str, Path]] = []
    appdata = os.environ.get("APPDATA")
    if appdata:
        copilot = Path(appdata) / "Code" / "User" / "globalStorage" / "github.copilot-chat" / "session-store.db"
        cursor = Path(appdata) / "Cursor" / "User" / "globalStorage" / "state.vscdb"
        if copilot.is_file():
            roots.append(("github_copilot", copilot))
        if cursor.is_file():
            roots.append(("cursor", cursor))
    codex = Path.home() / ".codex" / "sessions"
    claude = Path.home() / ".claude" / "history.jsonl"
    if codex.is_dir():
        roots.append(("codex", codex))
    if claude.is_file():
        roots.append(("claude_code", claude.parent))
    return roots


@dataclass(frozen=True)
class TrayConfig:
    gateway_url: str
    token_file: Path | None
    project_root: Path | None
    queue: Path
    device_id: str = "device-local"
    interval_seconds: int = 15
    authorized_roots: list[Path] = field(default_factory=list)
    vscode_state_path: Path | None = None
    ai_session_roots: list[tuple[str, Path]] = field(default_factory=list)
    browser_allowlist: set[str] = field(default_factory=set)
    browser_policy_revision: int = 0
    application_capture_enabled: bool = True
    application_capture_policy_revision: int = 0
    dlp_rules: dict = field(default_factory=dict)
    visual_studio_event_log: Path | None = None
    credential_file: Path | None = None
    project_roots: list[ProjectSource] = field(default_factory=list)
    heartbeat_seconds: int = 60
    show_status_on_first_run: bool = False
    subject_id: str = "local-user"
    tenant_id: str = "local-default"
    config_path: Path | None = None
    project_revision: int = 0
    control_credential_file: Path | None = None
    local_view_port: int = 15473

    @classmethod
    def from_file(cls, path: str | Path) -> "TrayConfig":
        config_path = Path(path).expanduser().resolve()
        raw = config_path.read_text(encoding="utf-8-sig")
        try:
            data = json.loads(raw)
        except json.JSONDecodeError:
            repaired = re.sub(r"(?<!\\)\\(?!\\)", r"\\\\", raw)
            data = json.loads(repaired)
            config_path.write_text(json.dumps(data, ensure_ascii=True, indent=2), encoding="utf-8", newline="\n")
        if data.get("core_version") != __version__:
            data["core_version"] = __version__
            config_path.write_text(json.dumps(data, ensure_ascii=True, indent=2), encoding="utf-8", newline="\n")
        if not data.get("device_id"):
            data["device_id"] = default_device_id()
            config_path.write_text(json.dumps(data, ensure_ascii=True, indent=2), encoding="utf-8", newline="\n")
        detected_ai_roots = [
            {"tool": tool, "path": str(path.expanduser().resolve())}
            for tool, path in default_ai_session_roots()
        ]
        configured_ai_roots = data.get("ai_session_roots")
        if not isinstance(configured_ai_roots, list):
            data["ai_session_roots"] = detected_ai_roots
            config_path.write_text(json.dumps(data, ensure_ascii=True, indent=2), encoding="utf-8", newline="\n")
        else:
            configured_tools = {
                str(item.get("tool")) for item in configured_ai_roots
                if isinstance(item, dict) and item.get("tool")
            }
            additions = [item for item in detected_ai_roots if item["tool"] not in configured_tools]
            if additions:
                data["ai_session_roots"] = configured_ai_roots + additions
                config_path.write_text(json.dumps(data, ensure_ascii=True, indent=2), encoding="utf-8", newline="\n")
        if not data.get("vscode_state_path"):
            data["vscode_state_path"] = str(VSCodeAdapter.default_state_path().expanduser().resolve())
            config_path.write_text(json.dumps(data, ensure_ascii=True, indent=2), encoding="utf-8", newline="\n")
        required = ("gateway_url", "queue")
        missing = [key for key in required if not data.get(key)]
        if not data.get("token_file") and not data.get("credential_file"):
            missing.append("credential_file")
        if missing:
            raise ValueError(f"missing tray configuration: {', '.join(missing)}")
        configured_project_root = Path(data["project_root"]).expanduser().resolve() if data.get("project_root") else None
        project_root = configured_project_root if configured_project_root is not None and configured_project_root.is_dir() else None
        default_roots = [str(project_root)] if project_root else []
        roots = [candidate for item in data.get("authorized_roots", default_roots) if (candidate := Path(item).expanduser().resolve()).is_dir()]
        project_roots = []
        for item in data.get("project_roots", []):
            if not isinstance(item, dict) or item.get("vcs") not in {"git", "svn"} or item.get("state", "active") != "active" or not item.get("path"):
                continue
            item_path = Path(item["path"]).expanduser().resolve()
            if item_path.is_dir():
                project_roots.append(ProjectSource(item_path, item["vcs"]))
        if "project_roots" not in data and not project_roots and project_root is not None:
            vcs = "svn" if (project_root / ".svn").is_dir() else "git"
            project_roots = [ProjectSource(project_root, vcs)]
        active_paths = {item.path for item in project_roots}
        if project_root not in active_paths:
            project_root = project_roots[0].path if project_roots else None
        if project_root != configured_project_root:
            if project_root is None:
                data.pop("project_root", None)
            else:
                data["project_root"] = str(project_root)
            config_path.write_text(json.dumps(data, ensure_ascii=True, indent=2), encoding="utf-8", newline="\n")
        return cls(
            gateway_url=str(data["gateway_url"]).rstrip("/"),
            token_file=Path(data["token_file"]).expanduser().resolve() if data.get("token_file") else None,
            project_root=project_root,
            queue=Path(data["queue"]).expanduser().resolve(),
            device_id=str(data.get("device_id", "device-local")),
            interval_seconds=max(5, int(data.get("interval_seconds", 15))),
            authorized_roots=roots,
            vscode_state_path=Path(data["vscode_state_path"]).expanduser().resolve(),
            ai_session_roots=[(str(item["tool"]), Path(item["path"]).expanduser().resolve()) for item in data.get("ai_session_roots", []) if isinstance(item, dict) and item.get("tool") and item.get("path")],
            browser_allowlist={str(domain).casefold() for domain in data.get("browser_allowlist", []) if domain},
            browser_policy_revision=max(0, int(data.get("browser_policy_revision", 0))),
            application_capture_enabled=bool(data.get("application_capture_enabled", True)),
            application_capture_policy_revision=max(0, int(data.get("application_capture_policy_revision", 0))),
            dlp_rules=data.get("dlp_rules", {}) if isinstance(data.get("dlp_rules", {}), dict) else {},
            visual_studio_event_log=Path(data["visual_studio_event_log"]).expanduser().resolve() if data.get("visual_studio_event_log") else None,
            credential_file=Path(data["credential_file"]).expanduser().resolve() if data.get("credential_file") else None,
            project_roots=project_roots,
            heartbeat_seconds=max(30, int(data.get("heartbeat_seconds", 60))),
            show_status_on_first_run=bool(data.get("show_status_on_first_run", False)),
            subject_id=str(data.get("subject_id", "local-user")),
            tenant_id=str(data.get("tenant_id", "local-default")),
            config_path=config_path,
            project_revision=int(data.get("project_revision", 0)),
            control_credential_file=Path(data["control_credential_file"]).expanduser().resolve() if data.get("control_credential_file") else None,
            local_view_port=max(0, int(data.get("local_view_port", 15473))),
        )


def event_from_record(record: dict, config: TrayConfig, source: str, project_path: str | Path | None = None):
    from .events import AetherisEvent

    raw_payload = record.get("payload", {})
    if not isinstance(raw_payload, dict):
        raise ValueError("record payload must be an object")
    payload = dict(raw_payload)
    hint = raw_payload.get("project_path") or raw_payload.get("project") or raw_payload.get("cwd")
    report = record.get("redaction_report", {"rules": [], "replacement_count": 0})
    record_provenance = record.get("provenance", {}) if isinstance(record.get("provenance", {}), dict) else {}
    source_key = json.dumps([record.get("event_type"), raw_payload, record_provenance], ensure_ascii=True, sort_keys=True, separators=(",", ":"))
    legacy_event_id = "event-" + hashlib.sha256(source_key.encode("utf-8")).hexdigest()[:32]
    event_id = legacy_event_id
    supersedes_event_id = None
    is_ai = source.startswith("core.ai.") or str(record.get("event_type", "")).startswith("ai.")
    if is_ai:
        tool = str(raw_payload.get("tool") or source.removeprefix("core.ai.") or "ai")
        attribution = attribute_ai_project(tool, hint, config.authorized_roots, namespace=config.tenant_id)
        for field_name in ("project", "project_path", "cwd"):
            payload.pop(field_name, None)
        payload["project_label"] = attribution.project_label
        payload["project_attribution"] = attribution.attribution
        project_id = attribution.project_id
        provenance = {**record_provenance, "project_label": attribution.project_label, "project_attribution": attribution.attribution}
        if attribution.project_root is not None:
            provenance["project_root"] = str(attribution.project_root)
        else:
            event_id = "event-" + hashlib.sha256((source_key + "\x00tool_fallback_v1").encode("utf-8")).hexdigest()[:32]
            supersedes_event_id = legacy_event_id
    else:
        project = ProjectResolver().resolve(hint or project_path, config.authorized_roots) if (hint or project_path) else None
        if hint and project is None:
            raise ValueError("event project hint is not an authorized project root")
        project = project or (ProjectResolver().resolve(config.project_root, config.authorized_roots) if config.project_root else None)
        if project is None:
            raise ValueError("configured project root is not authorized")
        project_id = project.project_id
        provenance = {"project_root": str(project.root), **record_provenance}
    return AetherisEvent.create(
        record["event_type"],
        tenant_id=config.tenant_id,
        subject_id=config.subject_id,
        device_id=config.device_id,
        project_id=project_id,
        session_id=str(payload.get("session_id") or f"session-{project_id}"),
        source=source,
        source_version=__version__,
        payload=payload,
        redaction_report=report,
        processing_grants=["server_ingest"],
        occurred_at=_record_occurred_at(payload),
        event_id=event_id,
        correlation_id=event_id,
        supersedes_event_id=supersedes_event_id,
        provenance=provenance,
    )


def _record_occurred_at(payload: dict) -> str | None:
    value = payload.get("timestamp")
    if not isinstance(value, str) or not value:
        return None
    try:
        parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError:
        return None
    return value if parsed.tzinfo is not None else None


def events_from_records(records: list[tuple], config: TrayConfig) -> list:
    events = []
    for item in records:
        record, source = item[0], item[1]
        project_path = item[2] if len(item) > 2 else None
        try:
            events.append(event_from_record(record, config, source, project_path))
        except ValueError:
            continue
    return events


def record_fingerprint(record: dict, source: str, project_path: str | Path | None, observed_at: datetime | None = None) -> str:
    parts: list[object] = [record, source, str(project_path)]
    if str(record.get("event_type", "")).startswith("browser."):
        timestamp = (observed_at or datetime.now(timezone.utc)).timestamp()
        parts.append(int(timestamp // 300))
    return hashlib.sha256(json.dumps(parts, sort_keys=True, ensure_ascii=True).encode("utf-8")).hexdigest()


class TrayCore:
    def __init__(self, config: TrayConfig):
        self.config = config
        self.project_registry = ProjectRegistry(config.config_path) if config.config_path else None
        self.queue = LocalQueue(config.queue)
        token = CredentialStore(config.credential_file).read() if config.credential_file else _read_token_file(str(config.token_file))
        if not token:
            raise ValueError("device token file is empty or unreadable")
        self.client = GatewayClient(config.gateway_url, token)
        self.control_token = CredentialStore(config.control_credential_file).read() if config.control_credential_file else secrets.token_urlsafe(32)
        identity_root = config.config_path.parent if config.config_path else config.queue.parent.parent / "config"
        self.project_identity_key_sync = ProjectIdentityKeySync(
            self.client,
            ProjectIdentityKeyStore(identity_root / "project-identity.credential"),
            ProjectIdentityKeyStore(identity_root / "project-root.credential"),
        )
        self.project_identity_keys = None
        self.project_sync = ProjectSync(self.project_registry, self.client) if self.project_registry else None
        self.discoverer = ProcessDiscoverer()
        self.consent = ConsentRegistry(allowed_names={
            "code.exe", "devenv.exe", "powershell.exe", "windowsterminal.exe",
            "codex.exe", "cursor.exe", "claude.exe", "git.exe", "node.exe",
        })
        self.process_consent = ProcessConsentCoordinator(
            config.queue.parent / "process-consent.db",
            user_sid=default_user_identifier(),
        )
        self.process_identity_provider = NativeProcessIdentityProvider()
        self.process_lineage_provider = NativeProcessLineageProvider()
        self.project_resolver = ProjectResolver()
        self.git_adapter = GitAdapter()
        self.svn_adapter = SvnAdapter()
        collector_state = config.queue.parent / "collector-state"
        self._server_sync_path = collector_state / "server-sync.json"
        self.command_privacy = load_or_create_command_privacy(config.queue.parent.parent / "config" / "command-fingerprint.credential")
        self.terminal_adapter = TerminalAdapter(command_privacy=self.command_privacy)
        self.command_correlator = CommandOriginCorrelator()
        self._suppressed_duplicate_terminal = 0
        self.vscode_adapter = VSCodeAdapter(config.vscode_state_path, project_roots=config.authorized_roots)
        self.ai_adapters = []
        for tool, path in config.ai_session_roots:
            if tool == "github_copilot":
                self.ai_adapters.append(CopilotSessionAdapter(path))
            elif tool == "cursor":
                self.ai_adapters.append(CursorSessionAdapter(path))
            else:
                self.ai_adapters.append(AISessionAdapter(path, tool, checkpoint_path=collector_state / f"{tool}.json", command_privacy=self.command_privacy))
        solutions = sorted(solution for project in config.project_roots for solution in project.path.glob("*.sln"))
        self.visual_studio_adapters = [VisualStudioAdapter(path) for path in solutions]
        self.visual_studio_event_adapter = VisualStudioEventAdapter(config.visual_studio_event_log) if config.visual_studio_event_log else None
        self.browser_ocr = OcrCapture(NodeTesseractEngine(["eng", "chi_sim"])) if config.browser_allowlist else None
        self.application_capture_policy = ApplicationCapturePolicy()
        self.application_window_inspector = ApplicationWindowInspector(identity_provider=self.process_identity_provider)
        self.application_ocr = ApplicationOcrAdapter(
            ClientAreaCapture(),
            OcrCapture(NodeTesseractEngine(["eng", "chi_sim"])),
            DlpMatcher(config.dlp_rules),
            self.command_privacy,
        )
        self.application_executor = ThreadPoolExecutor(max_workers=1, thread_name_prefix="aetheris-application-ocr")
        self._application_future = None
        self._application_future_identity = ""
        self._application_future_project = ""
        self._application_future_at = None
        self._application_fallback_counts = {"eligible": 0, "captured": 0, "parsed": 0, "emitted": 0, "skipped": 0, "failed": 0}
        self._seen_processes: set[tuple[int, str]] = set()
        self._seen_records: set[str] = set()
        self._stop = threading.Event()
        self._stopped = False
        self.status_path = config.queue.parent / "core-status.json"
        self.status = CoreStatus(self.status_path, version=__version__, device_id=config.device_id, gateway_url=config.gateway_url)
        self.adapter_health = AdapterHealthRegistry(config.queue.parent / "adapter-health.json")
        vscode_installation_state = identity_root / "vscode-integration.json"
        self.vscode_installation = VSCodeInstallation(
            vscode_installation_state,
            identity_root.parent / "integrations" / "AetherisVSCode.vsix",
        )
        initial_vscode_state = self.vscode_installation.inspect()
        self.vscode_install_executor = ThreadPoolExecutor(max_workers=1, thread_name_prefix="aetheris-vscode-install")
        self._vscode_install_future = None
        self.vscode_bridge_processor = VSCodeBridgeProcessor(
            self.queue,
            self.config,
            snapshot_callback=self._on_vscode_snapshot,
        )
        if initial_vscode_state.state in {"install_declined", "paused_by_user", "not_installed", "pending_install", "error"}:
            self.vscode_bridge_processor.set_desired_state("paused_by_user")
        self.vscode_bridge = VSCodeBridgeServer(self.vscode_bridge_processor)
        self._publish_vscode_installation_state()
        self.status.update_browser_policy(
            state="enabled" if config.browser_allowlist else "disabled",
            revision=config.browser_policy_revision,
            domain_count=len(config.browser_allowlist),
        )
        self.log_path = config.queue.parent.parent / "logs" / "core.log"
        self.log_path.parent.mkdir(parents=True, exist_ok=True)
        self.registration_state = "pending"
        self._heartbeat_backoff = HeartbeatBackoff()

    def sync_project_identity_keys(self) -> bool:
        try:
            self.project_identity_keys = self.project_identity_key_sync.sync()
            self.status.update_project_identity("available", key_version=self.project_identity_keys.key_version)
            return True
        except Exception as exc:
            error_code = type(exc).__name__
            self.status.update_project_identity("error", error=error_code)
            self._log(f"project_identity state=error error={error_code}")
            return False

    def sync_projects(self) -> bool:
        if self.project_sync is None or self.project_identity_keys is None:
            self.status.update_project_sync("waiting")
            return False
        try:
            result = self.project_sync.sync(self.project_identity_keys)
            if result is None:
                return True
            self.status.update_project_sync(
                "registered",
                registry_revision=result.registry_revision,
                project_count=len(result.projects),
            )
            return True
        except Exception as exc:
            error_code = type(exc).__name__
            self.status.update_project_sync("error", error=error_code)
            self._log(f"project_sync state=error error={error_code}")
            return False

    def sync_adapter_health(self) -> bool:
        try:
            snapshots = self.adapter_health.snapshot()
            if not snapshots:
                return True
            self.client.upload_adapter_health(snapshots)
            return True
        except Exception as exc:
            self._log(f"adapter_health_upload state=error error={type(exc).__name__}")
            return False

    def start_optional_integrations(self) -> bool:
        future = getattr(self, "_vscode_install_future", None)
        if future is not None:
            if not future.done():
                return False
            try:
                future.result()
            except Exception:
                pass
            self._vscode_install_future = None
        if not self.vscode_installation.should_auto_install():
            return False
        self._vscode_install_future = self.vscode_install_executor.submit(self._install_vscode_extension)
        return True

    def _install_vscode_extension(self):
        try:
            result = self.vscode_installation.install()
        except Exception:
            result = None
        if result is not None and result.state == "awaiting_activation":
            self.vscode_bridge_processor.set_desired_state("active")
        else:
            self.vscode_bridge_processor.set_desired_state("paused_by_user")
        self._publish_vscode_installation_state()
        return result

    def _on_vscode_snapshot(self, snapshot: dict) -> None:
        installed = self.vscode_installation.inspect()
        published = dict(snapshot)
        if installed.state in {"install_declined", "paused_by_user", "not_installed"}:
            published["component_state"] = installed.state
        elif snapshot.get("component_state") == "active":
            self.vscode_installation.mark_active(str(snapshot.get("component_version", "")))
        self.adapter_health.update_component("vscode_extension", published)

    def _publish_vscode_installation_state(self) -> None:
        state = self.vscode_installation.inspect()
        snapshot = self.vscode_bridge_processor.snapshot()
        snapshot.update({
            "component_state": state.state,
            "component_version": state.extension_version,
        })
        self.adapter_health.update_component("vscode_extension", snapshot)

    def vscode_status_snapshot(self) -> dict:
        installed = self.vscode_installation.inspect()
        snapshot = self.vscode_bridge_processor.snapshot()
        if not snapshot.get("last_component_heartbeat_at") or installed.state in {
            "install_declined", "paused_by_user", "not_installed", "pending_install",
        }:
            snapshot["component_state"] = installed.state
            snapshot["component_version"] = installed.extension_version
        snapshot["reason_code"] = installed.reason_code
        return snapshot

    def vscode_recent_events(self) -> list[dict]:
        project_names = {}
        if self.project_registry is not None:
            project_names = {
                item.local_project_id: item.display_name
                for item in self.project_registry.load().projects
            }
        projected = []
        for event in self.queue.history(limit=200):
            if event.get("source") != "core.vscode.extension":
                continue
            payload = event.get("payload") if isinstance(event.get("payload"), dict) else {}
            project_id = str(event.get("project_id", ""))
            projected.append({
                "event_type": str(event.get("event_type", "")),
                "occurred_at": str(event.get("occurred_at", "")),
                "project_id": project_id,
                "project_name": project_names.get(project_id, project_id or "未归属"),
                "relative_path": str(payload.get("relative_path", "")),
                "language_id": str(payload.get("language_id", "")),
            })
            if len(projected) >= 20:
                break
        return projected

    def control_vscode(self, operation: str):
        if operation == "clear-cache":
            self.vscode_bridge_processor.request_clear_cache()
            return None
        if operation == "install":
            result = self.vscode_installation.install()
            self.vscode_bridge_processor.set_desired_state("active")
        elif operation == "enable":
            result = self.vscode_installation.enable()
            self.vscode_bridge_processor.set_desired_state("active")
        elif operation == "pause":
            result = self.vscode_installation.pause()
            self.vscode_bridge_processor.set_desired_state("paused_by_user")
        elif operation == "uninstall":
            self.vscode_bridge_processor.set_desired_state("paused_by_user")
            result = self.vscode_installation.uninstall()
        else:
            raise ValueError("VS Code 操作无效")
        if result.state == "error":
            raise RuntimeError(result.reason_code or "vscode_operation_failed")
        self._publish_vscode_installation_state()
        return result

    def reload_projects_if_changed(self) -> bool:
        if self.project_registry is None:
            return False
        snapshot = self.project_registry.load()
        if snapshot.revision == self.config.project_revision:
            return False
        active = [item for item in snapshot.projects if item.state == "active" and item.vcs in {"git", "svn"} and item.path.is_dir()]
        project_roots = [ProjectSource(item.path, item.vcs) for item in active]
        authorized_roots = [item.path for item in snapshot.projects if item.path.is_dir()]
        project_root = project_roots[0].path if project_roots else None
        self.config = replace(
            self.config,
            project_root=project_root,
            project_roots=project_roots,
            authorized_roots=authorized_roots,
            project_revision=snapshot.revision,
        )
        solutions = sorted(solution for project in project_roots for solution in project.path.glob("*.sln"))
        self.visual_studio_adapters = [VisualStudioAdapter(path) for path in solutions]
        if getattr(self, "vscode_adapter", None):
            self.vscode_adapter.update_project_roots(authorized_roots)
        if getattr(self, "vscode_bridge_processor", None):
            self.vscode_bridge_processor.config = self.config
        return True

    def _log(self, message: str) -> None:
        with self.log_path.open("a", encoding="utf-8") as handle:
            handle.write(f"{datetime.now(timezone.utc).isoformat()} {message}\n")

    def _update_queue_status(self) -> None:
        stats = self.queue.stats()
        self.status.update_queue(queued=stats["queued_count"], retrying=stats["inflight_count"], rejected=stats["rejected_count"])

    def apply_browser_policy(self, policy: dict) -> bool:
        revision = int(policy.get("revision", 0))
        if revision < 0:
            raise ValueError("browser policy revision must be non-negative")
        if revision < self.config.browser_policy_revision:
            self.status.update_browser_policy(
                state="enabled" if self.config.browser_allowlist else "disabled",
                revision=self.config.browser_policy_revision,
                domain_count=len(self.config.browser_allowlist),
            )
            return False
        raw_domains = policy.get("allowed_domains", [])
        if not isinstance(raw_domains, list) or any(not isinstance(value, str) or not value.strip() for value in raw_domains):
            raise ValueError("browser policy domains are invalid")
        enabled = policy.get("enabled") is True
        domains = {value.strip().casefold().rstrip(".") for value in raw_domains} if enabled else set()
        if enabled and not domains:
            raise ValueError("enabled browser policy requires domains")
        if revision == self.config.browser_policy_revision and domains == self.config.browser_allowlist:
            self.status.update_browser_policy(state="enabled" if domains else "disabled", revision=revision, domain_count=len(domains))
            return False

        if self.config.config_path:
            data = json.loads(self.config.config_path.read_text(encoding="utf-8-sig"))
            data["browser_allowlist"] = sorted(domains)
            data["browser_policy_revision"] = revision
            temporary = self.config.config_path.with_suffix(self.config.config_path.suffix + ".tmp")
            temporary.write_text(json.dumps(data, ensure_ascii=True, indent=2), encoding="utf-8", newline="\n")
            temporary.replace(self.config.config_path)

        self.config = replace(self.config, browser_allowlist=domains, browser_policy_revision=revision)
        self.browser_ocr = OcrCapture(NodeTesseractEngine(["eng", "chi_sim"])) if domains else None
        self.status.update_browser_policy(state="enabled" if domains else "disabled", revision=revision, domain_count=len(domains))
        return True

    def sync_browser_policy(self) -> bool:
        try:
            return self.apply_browser_policy(self.client.browser_policy())
        except Exception as exc:
            domains = self.config.browser_allowlist
            self.status.update_browser_policy(
                state="enabled" if domains else "disabled",
                revision=self.config.browser_policy_revision,
                domain_count=len(domains),
                error=str(exc),
            )
            self._log(f"browser_policy_sync error={type(exc).__name__}")
            return False

    def apply_application_capture_policy(self, policy: dict) -> bool:
        revision = int(policy.get("revision", -1))
        if revision < 0:
            raise ValueError("application capture policy revision must be non-negative")
        if revision < self.config.application_capture_policy_revision:
            return False
        enabled = policy.get("enabled") is True
        if revision == self.config.application_capture_policy_revision and enabled == self.config.application_capture_enabled:
            return False
        if self.config.config_path:
            data = json.loads(self.config.config_path.read_text(encoding="utf-8-sig"))
            data["application_capture_enabled"] = enabled
            data["application_capture_policy_revision"] = revision
            temporary = self.config.config_path.with_suffix(self.config.config_path.suffix + ".tmp")
            temporary.write_text(json.dumps(data, ensure_ascii=True, indent=2), encoding="utf-8", newline="\n")
            temporary.replace(self.config.config_path)
        self.config = replace(self.config, application_capture_enabled=enabled, application_capture_policy_revision=revision)
        if not enabled:
            future = getattr(self, "_application_future", None)
            if future is not None:
                future.cancel()
        return True

    def sync_application_capture_policy(self) -> bool:
        try:
            return self.apply_application_capture_policy(self.client.application_capture_policy())
        except Exception:
            self._log("application_capture_policy_sync error=policy_unavailable")
            return False

    def _upload_events(self, events) -> int:
        for event in events:
            self.queue.enqueue(event)
        accepted = 0
        while claimed := self.queue.claim_batch(100):
            try:
                results = self.client.send(claimed)
            except Exception:
                self.queue.release_with_backoff([event.event_id for event in claimed])
                raise
            for event, result in zip(claimed, results):
                if result["status"] in {"accepted", "duplicate"}:
                    self.queue.ack([event.event_id])
                    accepted += result["status"] == "accepted"
                else:
                    self.queue.reject([event.event_id], result.get("reason", "server rejected event"))
        self._update_queue_status()
        return accepted

    def heartbeat_once(self) -> int:
        try:
            response = self.client.heartbeat()
            if response.get("status") != "online" or response.get("device_id") not in {None, self.config.device_id}:
                raise ValueError("heartbeat identity mismatch")
            self._sync_server_history(response)
            self.registration_state = "registered"
            self.status.update_registration("registered", heartbeat_at=datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"))
            self.sync_browser_policy()
            self.sync_application_capture_policy()
            if self.sync_project_identity_keys():
                self.sync_projects()
            self.sync_adapter_health()
            self.start_optional_integrations()
            return self._heartbeat_backoff.success()
        except HTTPError as exc:
            self.registration_state = "credential_invalid" if exc.code in {401, 403} else "pending"
            delay = 300 if exc.code in {401, 403} else self._heartbeat_backoff.failure()
            self.status.update_registration(self.registration_state, error=f"HTTP {exc.code}")
            self._log(f"heartbeat state={self.registration_state} error=http_{exc.code}")
            return delay
        except Exception as exc:
            self.registration_state = "pending"
            delay = self._heartbeat_backoff.failure()
            self.status.update_registration("pending", error=str(exc))
            self._log(f"heartbeat state=pending error={type(exc).__name__}")
            return delay

    def _sync_server_history(self, response: dict) -> None:
        """Detect a server reset and rewind local history checkpoints once."""
        try:
            event_count = int(response.get("server_event_count"))
            generation = str(response.get("data_generation") or "")
        except (TypeError, ValueError):
            return
        if event_count < 0 or not generation:
            return
        previous_count = None
        previous_generation = ""
        try:
            state = json.loads(self._server_sync_path.read_text(encoding="utf-8"))
            if isinstance(state, dict):
                previous_count = int(state.get("server_event_count"))
                previous_generation = str(state.get("data_generation") or "")
        except (OSError, TypeError, ValueError, json.JSONDecodeError):
            pass
        reset = previous_count is None or event_count < previous_count
        if reset:
            for adapter in getattr(self, "ai_adapters", []):
                reset_history = getattr(adapter, "reset_history", None)
                if reset_history:
                    reset_history()
            getattr(self, "_seen_records", set()).clear()
            if previous_count is None:
                self._log("history_resync reason=server_sync_state_missing")
            else:
                self._log("history_resync reason=server_event_count_decreased")
        state = {"version": 1, "server_event_count": event_count, "data_generation": generation}
        self._server_sync_path.parent.mkdir(parents=True, exist_ok=True)
        temporary = self._server_sync_path.with_suffix(self._server_sync_path.suffix + ".tmp")
        temporary.write_text(json.dumps(state, ensure_ascii=True, separators=(",", ":")), encoding="utf-8", newline="\n")
        temporary.replace(self._server_sync_path)

    def capture_once(self) -> None:
        self.reload_projects_if_changed()
        if not self.config.project_roots or self.config.project_root is None:
            self.status.update_capture("waiting_for_project", project_count=0)
            self._update_queue_status()
            return
        observations = self.discoverer.discover()
        self.status.update_capture("running", project_count=len(self.config.project_roots))
        captured = 0
        for observation in observations:
            key = (observation.pid, observation.name.casefold())
            if key in self._seen_processes:
                continue
            decision = self.consent.classify(observation)
            if decision.status != "allowed":
                identity = self.process_identity_provider.inspect(observation.pid, observation.name)
                outcome = self.process_consent.observe(identity)
                project_ref = ProjectResolver().resolve(self.config.project_root, self.config.authorized_roots) if self.config.project_root else None
                if not can_capture(outcome, project_ref.project_id if project_ref else ""):
                    continue
            if decision.status != "allowed" and outcome.state not in {"allow_global", "allow_project"}:
                continue
            lineage = self.process_lineage_provider.inspect(observation.pid, observation.name)
            process_payload = {"pid": observation.pid, "name": observation.name.casefold(), **lineage.to_payload()}
            result = run_capture_and_upload(
                observation,
                self.config.project_root,
                self.config.authorized_roots,
                self.queue,
                self.client,
                device_id=self.config.device_id,
                tenant_id=self.config.tenant_id,
                subject_id=self.config.subject_id,
                raw_payload=process_payload,
            )
            self._seen_processes.add(key)
            captured += result["accepted"]
        records: list[tuple] = []
        capture_started_at = datetime.now(timezone.utc)
        # Browser capture is foreground-sensitive; collect it before slower
        # repository/session adapters so a slow source cannot starve it.
        try:
            if self.browser_ocr:
                browser_record = collect_visible_window(
                    self.config.browser_allowlist,
                    self.browser_ocr,
                    DlpMatcher(self.config.dlp_rules),
                    health=self.adapter_health,
                )
                if browser_record:
                    records.append((browser_record, "core.browser", self.config.project_root))
        except Exception:
            self.adapter_health.fail("browser", "error", "emit", "browser_emit_failed")
        adapter_errors = []
        terminal_records: list[dict] = []
        for project in self.config.project_roots:
            try:
                self.adapter_health.begin(project.vcs, "source_discovery")
                adapter = self.git_adapter if project.vcs == "git" else self.svn_adapter
                records.extend((record, f"core.{project.vcs}", project.path) for record in adapter.collect(project.path))
                self.adapter_health.finish(project.vcs, "active", discovered=len(records), parsed=len(records))
            except Exception as exc:
                adapter_errors.append(f"{project.vcs}:{project.path.name}:{type(exc).__name__}")
                self.adapter_health.fail(project.vcs, "error", "parse", f"{project.vcs}_collect_failed")
        try:
            self.adapter_health.begin("terminal", "source_discovery")
            terminal_records = self.terminal_adapter.collect()
            records.extend((record, "core.terminal", self.config.project_root) for record in terminal_records)
            if self.vscode_adapter:
                self.adapter_health.begin("vscode", "source_discovery")
                vscode_records = self.vscode_adapter.collect()
                records.extend((record, "core.vscode", record.get("project_path") or self.config.project_root) for record in vscode_records)
                self.adapter_health.finish(
                    "vscode",
                    "active" if self.vscode_adapter.last_discovered else "idle",
                    discovered=self.vscode_adapter.last_discovered,
                    parsed=len(vscode_records),
                    detected_format=self.vscode_adapter.detected_format,
                    last_event_at=datetime.now(timezone.utc).isoformat().replace("+00:00", "Z") if vscode_records else None,
                )
            for adapter in self.ai_adapters:
                self.adapter_health.begin(adapter.tool, "source_discovery")
                ai_records = adapter.collect()
                records.extend((record, f"core.ai.{adapter.tool}", self.config.project_root) for record in ai_records)
                progress = getattr(adapter, "progress", None)
                if isinstance(progress, dict):
                    self.adapter_health.finish(
                        adapter.tool,
                        "active",
                        discovered=int(progress.get("total_files", 0)),
                        parsed=len(ai_records),
                        detected_format="jsonl",
                        total_files=int(progress.get("total_files", 0)),
                        covered_files=int(progress.get("covered_files", 0)),
                        pending_files=int(progress.get("pending_files", 0)),
                    )
                else:
                    self.adapter_health.finish(adapter.tool, "active", discovered=1, parsed=len(ai_records))
            for adapter in self.visual_studio_adapters:
                self.adapter_health.begin("visual_studio", "source_discovery")
                records.extend((record, "core.visual_studio", self.config.project_root) for record in adapter.collect())
                self.adapter_health.finish("visual_studio", "active", discovered=1, parsed=1)
            if self.visual_studio_event_adapter:
                records.extend((record, "core.visual_studio.events", self.config.project_root) for record in self.visual_studio_event_adapter.collect())
        except Exception as exc:
            adapter_errors.append(type(exc).__name__)
            self.adapter_health.fail("collector", "error", "emit", "collector_failed")
        correlated = self.command_correlator.filter(records, now=capture_started_at, project_roots=self.config.authorized_roots)
        records = correlated.records
        self._suppressed_duplicate_terminal += correlated.suppressed_count
        retained_terminal = sum(1 for record, source, *_ in records if source == "core.terminal" and record.get("event_type") == "terminal.command")
        self.adapter_health.finish(
            "terminal",
            "active" if terminal_records else "idle",
            discovered=len(terminal_records),
            parsed=retained_terminal,
            skipped=correlated.suppressed_count,
        )
        native_process_names = _native_process_names(records)
        captured += self.capture_application_fallback(observations, set(), capture_started_at, native_process_names=native_process_names)
        unique_records = []
        for record, source, project_path in records:
            fingerprint = record_fingerprint(record, source, project_path)
            if fingerprint in self._seen_records:
                continue
            unique_records.append((record, source, project_path))
            self._seen_records.add(fingerprint)
        derived_events = events_from_records(unique_records, self.config)
        captured += self._upload_events(derived_events)
        for adapter in self.ai_adapters:
            save_checkpoint = getattr(adapter, "save_checkpoint", None)
            if save_checkpoint:
                save_checkpoint()
        error = "; ".join(adapter_errors)
        self.status.update_capture("error" if error else "running", project_count=len(self.config.project_roots), error=error, capture_at=datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"), suppressed_duplicate_terminal=self._suppressed_duplicate_terminal)
        self._update_queue_status()

    @property
    def status_text(self) -> str:
        capture = self.status.snapshot()["capture"]["state"]
        return f"注册: {self.registration_state} | 采集: {capture} | 项目: {len(self.config.project_roots)}"

    def worker(self) -> None:
        next_heartbeat = 0.0
        next_capture = 0.0
        while not self._stop.is_set():
            now = time.monotonic()
            if now >= next_heartbeat:
                next_heartbeat = now + self.heartbeat_once()
            if now >= next_capture:
                try:
                    self.capture_once()
                except Exception as exc:
                    self.status.update_capture("error", project_count=len(self.config.project_roots), error=str(exc))
                    self._log(f"capture state=error error={type(exc).__name__}")
                next_capture = now + self.config.interval_seconds
            wait_for = max(0.2, min(next_heartbeat, next_capture) - time.monotonic())
            self._stop.wait(wait_for)

    def stop(self) -> None:
        if self._stopped:
            return
        self._stopped = True
        self._stop.set()
        self.vscode_bridge.stop()
        executor = getattr(self, "application_executor", None)
        if executor is not None:
            executor.shutdown(wait=False, cancel_futures=True)
        vscode_executor = getattr(self, "vscode_install_executor", None)
        if vscode_executor is not None:
            vscode_executor.shutdown(wait=False, cancel_futures=True)
        self.queue.close()
        self.process_consent.close()

    def capture_application_fallback(self, observations, native_event_keys: set[str], now: datetime, *, native_process_names: set[str] | None = None) -> int:
        if not self.config.application_capture_enabled:
            self._finish_application_health("disabled", "policy_disabled")
            return 0
        emitted = self._consume_application_fallback()
        window = self.application_window_inspector.inspect()
        if window is None:
            self._finish_application_health("idle", "window_unavailable")
            return emitted
        observation = next((item for item in observations if item.pid == window.pid), None)
        if observation is None:
            self._finish_application_health("idle", "window_unavailable")
            return emitted
        identity = self.process_identity_provider.inspect(observation.pid, observation.name)
        outcome = self.process_consent.observe(identity)
        identity_key = outcome.identity_key
        project = self.project_resolver.resolve(self.config.project_root, self.config.authorized_roots) if self.config.project_root else None
        project_id = project.project_id if project else ""
        if identity_key in native_event_keys or window.process_name.casefold() in (native_process_names or set()):
            self.application_capture_policy.mark_native_event(identity_key, now)
        candidate = CaptureCandidate(
            identity_key=identity_key,
            process_name=window.process_name,
            consent_state=outcome.state if can_capture(outcome, project_id) else "pending",
            project_id=project_id,
            foreground=True,
            visible=window.visible,
            minimized=window.minimized,
            has_password_control=window.has_password_control,
            has_client_area=window.client_rect[2] > window.client_rect[0] and window.client_rect[3] > window.client_rect[1],
            queue_watermark=self.queue.stats()["watermark"],
            ocr_busy=self._application_future is not None,
            now=now,
        )
        decision = self.application_capture_policy.evaluate(candidate)
        if not decision.eligible:
            self._application_fallback_counts["skipped"] += 1
            self._finish_application_health("idle", decision.reason_code)
            return emitted
        if self._application_future is None:
            self._application_fallback_counts["eligible"] += 1
            capture_candidate = ApplicationCaptureCandidate(identity_key, project_id, now, window)
            self._application_future = self.application_executor.submit(self.application_ocr.collect, capture_candidate)
            self._application_future_identity = identity_key
            self._application_future_project = project_id
            self._application_future_at = now
        return emitted + self._consume_application_fallback()

    def _consume_application_fallback(self) -> int:
        future = getattr(self, "_application_future", None)
        if future is None or not future.done():
            return 0
        identity_key = self._application_future_identity
        project_id = self._application_future_project
        occurred_at = self._application_future_at or datetime.now(timezone.utc)
        self._application_future = None
        if not self.config.application_capture_enabled:
            self._application_fallback_counts["skipped"] += 1
            self._finish_application_health("disabled", "policy_disabled")
            return 0
        try:
            result = future.result()
        except Exception:
            result = None
        if result is None or result.record is None:
            reason_code = result.reason_code if result is not None else "ocr_failed"
            if reason_code in {"duplicate_window_content", "dlp_blocked"}:
                self._application_fallback_counts["skipped"] += 1
            else:
                self._application_fallback_counts["failed"] += 1
                self.application_capture_policy.mark_result(identity_key, "failure", occurred_at)
            self._finish_application_health("idle" if reason_code == "duplicate_window_content" else "error", reason_code)
            return 0
        from .events import AetherisEvent

        event = AetherisEvent.create(
            "application.activity",
            tenant_id=self.config.tenant_id,
            subject_id=self.config.subject_id,
            device_id=self.config.device_id,
            project_id=project_id,
            session_id=f"application-{identity_key[:16]}",
            source="core.application.ocr",
            source_version=__version__,
            payload=result.record["payload"],
            redaction_report=result.record.get("redaction_report", {"rules": [], "replacement_count": 0}),
            processing_grants=["server_ingest"],
            occurred_at=occurred_at.isoformat().replace("+00:00", "Z"),
            provenance={"capture_method": "ocr_fallback"},
        )
        self.queue.enqueue(event)
        self.application_capture_policy.mark_result(identity_key, "success", occurred_at)
        self._application_fallback_counts["captured"] += 1
        self._application_fallback_counts["parsed"] += 1
        self._application_fallback_counts["emitted"] += 1
        self._finish_application_health("active", "captured", event.occurred_at)
        return 1

    def _finish_application_health(self, state: str, reason_code: str, last_event_at: str | None = None) -> None:
        counts = self._application_fallback_counts
        self.adapter_health.finish(
            "application_ocr_fallback",
            state,
            discovered=counts["eligible"],
            parsed=counts["parsed"],
            skipped=counts["skipped"],
            failed=counts["failed"],
            last_event_at=last_event_at,
            detected_format=f"ocr_fallback:{reason_code}",
        )


def _native_process_names(records: list[tuple]) -> set[str]:
    names = set()
    for record, source, *_ in records:
        if not isinstance(record, dict):
            continue
        if source.startswith("core.vscode"):
            names.add("code.exe")
        elif source.startswith("core.visual_studio"):
            names.add("devenv.exe")
        elif source == "core.terminal":
            names.update({"powershell.exe", "pwsh.exe", "cmd.exe", "windowsterminal.exe"})
        elif source.startswith("core.ai."):
            tool = source.removeprefix("core.ai.")
            names.add({"codex": "codex.exe", "claude_code": "claude.exe", "cursor": "cursor.exe"}.get(tool, ""))
    names.discard("")
    return names


def _show_error(message: str) -> None:
    if hasattr(ctypes, "windll"):
        ctypes.windll.user32.MessageBoxW(0, message, "Aetheris Core", 0x10)
    else:
        print(message)


def run_tray(config_path: str | Path | None = None, service_client: ServiceClient | None = None) -> bool:
    try:
        import pystray
        from PIL import Image, ImageDraw
    except ImportError as exc:
        raise RuntimeError("Tray runtime dependencies are missing; rebuild the bundled executable") from exc

    resolved_config_path = Path(config_path).expanduser().resolve() if config_path else default_config_path()
    if not resolved_config_path.is_file():
        raise ValueError("缺少 Core 配置，请先运行 AetherisSetup.exe")
    config = TrayConfig.from_file(resolved_config_path)
    app = TrayCore(config)
    image = Image.new("RGBA", (64, 64), (24, 75, 105, 255))
    draw = ImageDraw.Draw(image)
    draw.rounded_rectangle((8, 8, 56, 56), radius=10, fill=(44, 155, 166, 255))
    draw.text((23, 20), "A", fill=(255, 255, 255, 255))

    def status_text(_item):
        return app.status_text

    def capture_now(_icon, _item):
        threading.Thread(target=app.capture_once, daemon=True).start()

    def heartbeat_now(_icon=None, _item=None):
        threading.Thread(target=app.heartbeat_once, daemon=True).start()

    user_exit = threading.Event()
    ipc_stop = threading.Event()

    def exit_app(icon, _item):
        if service_client:
            service_client.normal_exit()
        user_exit.set()
        app.stop()
        icon.stop()

    def exit_from_local():
        if service_client:
            service_client.normal_exit()
        user_exit.set()
        app.stop()
        icon.stop()

    def combined_status():
        value = app.status.snapshot()
        value["adapters"] = app.adapter_health.snapshot()
        value["service"] = service_client.status_snapshot() if service_client else {"state": "standalone"}
        return value

    local_server = create_local_view_server(
        app.queue.history,
        combined_status,
        app.heartbeat_once,
        app.capture_once,
        exit_from_local,
        process_consent_provider=app.process_consent.list_all,
        process_consent_decision=lambda body: app.process_consent.decide_key(
            str(body.get("identity_key", "")),
            str(body.get("decision", "")),
            body.get("project_ids", []),
        ),
        process_consent_reset=app.process_consent.reset_key,
        local_episode_provider=lambda: build_local_episodes(
            app.queue.history(limit=500),
            device_id=app.config.device_id,
            project_names={item.local_project_id: item.display_name for item in app.project_registry.load().projects} if app.project_registry else {},
        ),
        vscode_status_provider=app.vscode_status_snapshot,
        vscode_events_provider=app.vscode_recent_events,
        vscode_action=app.control_vscode,
        project_registry=app.project_registry,
        control_token=app.control_token,
        port=config.local_view_port,
    )
    local_url = f"http://127.0.0.1:{local_server.server_port}/"
    app.status.set_local_view(local_url)
    icon = pystray.Icon(
        "aetheris-core",
        image,
        "Aetheris Core",
        menu=pystray.Menu(
            pystray.MenuItem(status_text, None, enabled=False),
            pystray.MenuItem("打开本地状态", lambda _icon, _item: webbrowser.open(local_url)),
            pystray.MenuItem("立即 heartbeat", heartbeat_now),
            pystray.MenuItem("立即采集", capture_now),
            pystray.MenuItem("打开数据目录", lambda _icon, _item: __import__("os").startfile(str(config.queue.parent))),
            pystray.MenuItem("退出", exit_app),
        ),
    )
    local_thread = threading.Thread(target=local_server.serve_forever, daemon=True)
    local_thread.start()
    worker = threading.Thread(target=app.worker, daemon=True)
    app.vscode_bridge.start()
    worker.start()
    ipc_thread = None
    _announce_ready_and_start_optional(app, service_client)
    if service_client:

        def service_heartbeat():
            while not ipc_stop.wait(30):
                try:
                    service_client.heartbeat()
                except ServiceUnavailable:
                    pass

        ipc_thread = threading.Thread(target=service_heartbeat, daemon=True)
        ipc_thread.start()
    try:
        def setup_icon(active_icon):
            active_icon.visible = True
            try:
                active_icon.notify("Aetheris Core 已启动，可从通知区域打开本地状态。", "Aetheris Core")
            except (NotImplementedError, OSError):
                pass
            if config.show_status_on_first_run:
                webbrowser.open(local_url)
                _mark_first_status_shown(resolved_config_path)
        icon.run(setup=setup_icon)
    finally:
        ipc_stop.set()
        app.stop()
        local_server.shutdown()
        local_server.server_close()
        local_thread.join(timeout=2)
        if ipc_thread:
            ipc_thread.join(timeout=2)
        if service_client:
            service_client.close()
    return user_exit.is_set()


def _announce_ready_and_start_optional(app, service_client) -> None:
    if service_client:
        service_client.ready()
    app.start_optional_integrations()


def parse_args(argv: list[str] | None = None):
    parser = argparse.ArgumentParser(description="Aetheris Core tray application")
    parser.add_argument("--version", action="version", version=__version__)
    parser.add_argument("--config")
    parser.add_argument("--supervised", action="store_true")
    parser.add_argument("--service-session", type=int)
    parser.add_argument("--standalone", action="store_true")
    return parser.parse_args(argv)


def _handle_existing_instance(config_path: Path, supervised: bool) -> None:
    if supervised:
        return
    url = existing_status_url(config_path)
    if url:
        webbrowser.open(url)
    else:
        _show_error("Aetheris Core 已在运行，请从 Windows 通知区域打开本地状态。")


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)
    resolved = Path(args.config).expanduser().resolve() if args.config else default_config_path()
    lifecycle = LifecycleLog(resolved.parent.parent / "logs" / "core.log")
    lifecycle.start(args.supervised)
    instance = SingleInstance()
    try:
        if not instance.acquire():
            _handle_existing_instance(resolved, args.supervised)
            lifecycle.stop("already_running")
            return 0
        service_client = None
        service_required = args.service_session is not None or (getattr(sys, "frozen", False) and not args.standalone)
        if service_required:
            service_client = ServiceClient.for_current_process(args.service_session)
            service_client.connect(timeout_seconds=10)
            if args.service_session is None:
                service_client.resume()
        if not run_tray(args.config, service_client):
            lifecycle.fatal("UnexpectedTrayExit")
            return 1
    except Exception as exc:
        lifecycle.fatal(type(exc).__name__)
        _show_error(str(exc))
        return 1
    finally:
        instance.close()
    lifecycle.stop("user_exit")
    return 0


def _mark_first_status_shown(config_path: Path) -> None:
    try:
        data = json.loads(config_path.read_text(encoding="utf-8-sig"))
        data["show_status_on_first_run"] = False
        temporary = config_path.with_suffix(config_path.suffix + ".tmp")
        temporary.write_text(json.dumps(data, ensure_ascii=True, indent=2), encoding="utf-8")
        temporary.replace(config_path)
    except (OSError, json.JSONDecodeError):
        return


if __name__ == "__main__":
    raise SystemExit(main())
