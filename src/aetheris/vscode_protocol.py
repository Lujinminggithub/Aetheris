from __future__ import annotations

import json
from dataclasses import dataclass
from datetime import datetime


MAX_FRAME_BYTES = 64 * 1024
MAX_BATCH_EVENTS = 100
PROTOCOL_VERSION = 1
EVENT_TYPES = {
    "ide.file_opened",
    "ide.file_edited",
    "ide.file_saved",
    "ide.file_closed",
    "ide.workspace_changed",
    "ide.extension_changed",
}
MESSAGE_TYPES = {"handshake", "heartbeat", "events"}
FORBIDDEN_FIELDS = {
    "source_text", "text", "content", "diff", "patch", "clipboard",
    "terminal_output", "inserted_text", "deleted_text", "file_content",
}
COMMON_EVENT_FIELDS = {"event_id", "event_type", "occurred_at", "session_id"}
EVENT_FIELDS = COMMON_EVENT_FIELDS | {
    "workspace_path", "file_path", "relative_path", "file_name", "extension",
    "language_id", "uri_scheme", "read_only", "change_count", "inserted_chars",
    "deleted_chars", "undo_count", "redo_count", "flush_reason",
    "edit_started_at", "edit_ended_at", "edit_duration_ms", "had_pending_edit",
    "workspace_kind", "added_workspace_paths", "removed_workspace_paths",
    "extension_id", "version", "previous_version", "change", "is_active",
    "previous_is_active",
}
ENVELOPE_FIELDS = {
    "version", "type", "session_id", "events", "extension_id",
    "extension_version", "vscode_version", "host_kind", "pending_events",
    "sent_events", "dropped_events",
    "component_state",
}


class BridgeProtocolError(ValueError):
    pass


@dataclass(frozen=True)
class BridgeEnvelope:
    message_type: str
    session_id: str
    events: tuple[dict, ...] = ()
    metadata: dict | None = None


def parse_bridge_frame(raw: bytes) -> BridgeEnvelope:
    if not isinstance(raw, bytes) or not 0 < len(raw) <= MAX_FRAME_BYTES:
        raise BridgeProtocolError("frame_size_invalid")
    try:
        value = json.loads(raw.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise BridgeProtocolError("json_invalid") from exc
    if not isinstance(value, dict):
        raise BridgeProtocolError("envelope_invalid")
    _reject_forbidden_fields(value)
    if set(value) - ENVELOPE_FIELDS:
        raise BridgeProtocolError("unknown_field")
    if value.get("version") != PROTOCOL_VERSION:
        raise BridgeProtocolError("version_unsupported")
    message_type = value.get("type")
    if message_type not in MESSAGE_TYPES:
        raise BridgeProtocolError("message_type_invalid")
    session_id = _bounded_string(value.get("session_id"), "session_id_invalid", 128)
    events: tuple[dict, ...] = ()
    if message_type == "events":
        raw_events = value.get("events")
        if not isinstance(raw_events, list) or not 0 < len(raw_events) <= MAX_BATCH_EVENTS:
            raise BridgeProtocolError("batch_size_invalid")
        events = tuple(_validate_event(item) for item in raw_events)
    elif "events" in value:
        raise BridgeProtocolError("events_not_allowed")
    metadata = {key: item for key, item in value.items() if key not in {"version", "type", "session_id", "events"}}
    _validate_metadata(metadata)
    return BridgeEnvelope(message_type=message_type, session_id=session_id, events=events, metadata=metadata)


def _validate_event(value: object) -> dict:
    if not isinstance(value, dict) or set(value) - EVENT_FIELDS:
        raise BridgeProtocolError("event_field_invalid")
    event_type = value.get("event_type")
    if event_type not in EVENT_TYPES:
        raise BridgeProtocolError("event_type_invalid")
    _bounded_string(value.get("event_id"), "event_id_invalid", 128)
    _parse_time(value.get("occurred_at"), "occurred_at_invalid")
    for key, item in value.items():
        if isinstance(item, str) and len(item) > (32768 if key.endswith("_path") else 512):
            raise BridgeProtocolError("event_field_too_long")
        if key.endswith("_chars") or key.endswith("_count") or key == "edit_duration_ms":
            if not isinstance(item, int) or isinstance(item, bool) or item < 0:
                raise BridgeProtocolError("event_count_invalid")
        if key in {"added_workspace_paths", "removed_workspace_paths"}:
            if not isinstance(item, list) or len(item) > 32 or any(not isinstance(path, str) or len(path) > 32768 for path in item):
                raise BridgeProtocolError("workspace_paths_invalid")
    return dict(value)


def _validate_metadata(metadata: dict) -> None:
    for key, value in metadata.items():
        if key in {"pending_events", "sent_events", "dropped_events"}:
            if not isinstance(value, int) or isinstance(value, bool) or value < 0:
                raise BridgeProtocolError("heartbeat_count_invalid")
        elif key == "component_state" and value not in {"active", "paused_by_user", "unsupported_remote_host"}:
            raise BridgeProtocolError("component_state_invalid")
        elif not isinstance(value, str) or not value or len(value) > 128:
            raise BridgeProtocolError("metadata_invalid")


def _reject_forbidden_fields(value: object) -> None:
    if isinstance(value, dict):
        for key, item in value.items():
            if str(key).casefold() in FORBIDDEN_FIELDS:
                raise BridgeProtocolError("forbidden_field")
            _reject_forbidden_fields(item)
    elif isinstance(value, list):
        for item in value:
            _reject_forbidden_fields(item)


def _bounded_string(value: object, reason: str, limit: int) -> str:
    if not isinstance(value, str) or not value or len(value) > limit:
        raise BridgeProtocolError(reason)
    return value


def _parse_time(value: object, reason: str) -> datetime:
    if not isinstance(value, str):
        raise BridgeProtocolError(reason)
    try:
        parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError as exc:
        raise BridgeProtocolError(reason) from exc
    if parsed.tzinfo is None:
        raise BridgeProtocolError(reason)
    return parsed
