from __future__ import annotations

from pathlib import Path

from .core import ProjectResolver
from .events import AetherisEvent
from .redaction import Redactor
from .version import __version__
from .vscode_protocol import EVENT_TYPES


FILE_EVENTS = {"ide.file_opened", "ide.file_edited", "ide.file_saved", "ide.file_closed"}
SAFE_FIELDS = {
    "relative_path", "file_name", "extension", "language_id", "uri_scheme", "read_only",
    "change_count", "inserted_chars", "deleted_chars", "undo_count", "redo_count",
    "flush_reason", "edit_started_at", "edit_ended_at", "edit_duration_ms", "had_pending_edit",
    "workspace_kind", "extension_id", "version", "previous_version", "change", "is_active",
    "previous_is_active",
}


class VSCodeEventRejected(ValueError):
    pass


def project_vscode_event(value: dict, config, *, session_id: str | None = None) -> AetherisEvent:
    event_type = str(value.get("event_type", ""))
    if event_type not in EVENT_TYPES:
        raise VSCodeEventRejected("event_type_invalid")
    root = _event_root(value, config, event_type)
    project = ProjectResolver().resolve(root, config.authorized_roots)
    if project is None:
        raise VSCodeEventRejected("path_not_authorized")
    payload = {key: item for key, item in value.items() if key in SAFE_FIELDS}
    if event_type in FILE_EVENTS:
        file_path = Path(str(value.get("file_path", ""))).expanduser().resolve()
        try:
            relative = file_path.relative_to(root).as_posix()
        except ValueError as exc:
            raise VSCodeEventRejected("path_not_authorized") from exc
        payload.update({
            "relative_path": relative,
            "file_name": file_path.name[:512],
            "extension": file_path.suffix.casefold()[:32],
            "uri_scheme": "file",
        })
    safe_payload, report = Redactor().redact(payload)
    event_id = str(value.get("event_id", ""))
    if not event_id:
        raise VSCodeEventRejected("event_id_invalid")
    return AetherisEvent.create(
        event_type,
        tenant_id=config.tenant_id,
        subject_id=config.subject_id,
        device_id=config.device_id,
        project_id=project.project_id,
        session_id=str(value.get("session_id") or session_id or f"vscode-{project.project_id}"),
        source="core.vscode.extension",
        source_version=__version__,
        payload=safe_payload,
        redaction_report=report,
        processing_grants=["server_ingest"],
        occurred_at=str(value.get("occurred_at", "")) or None,
        event_id=event_id,
        correlation_id=event_id,
        provenance={"capture_method": "vscode_extension", "bridge_protocol": 1},
    )


def _event_root(value: dict, config, event_type: str) -> Path:
    roots = [Path(root).expanduser().resolve() for root in config.authorized_roots]
    if event_type in FILE_EVENTS:
        raw_path = value.get("file_path")
        if not isinstance(raw_path, str) or not raw_path:
            raise VSCodeEventRejected("file_path_missing")
        file_path = Path(raw_path).expanduser().resolve()
        for root in sorted(roots, key=lambda item: len(item.parts), reverse=True):
            try:
                file_path.relative_to(root)
                return root
            except ValueError:
                continue
        raise VSCodeEventRejected("path_not_authorized")
    if event_type == "ide.workspace_changed":
        candidates = value.get("added_workspace_paths", []) or value.get("removed_workspace_paths", [])
        for candidate in candidates if isinstance(candidates, list) else []:
            try:
                path = Path(str(candidate)).expanduser().resolve()
            except OSError:
                continue
            if path in roots:
                return path
    fallback = config.project_root or (roots[0] if roots else None)
    if fallback is None:
        raise VSCodeEventRejected("project_unavailable")
    return Path(fallback).expanduser().resolve()
