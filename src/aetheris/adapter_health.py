from __future__ import annotations

import json
import threading
from datetime import datetime, timezone
from pathlib import Path


STATES = {
    "active", "idle", "disabled", "source_missing", "permission_denied",
    "format_changed", "dependency_missing", "error",
}
STAGES = {
    "foreground_window", "process_identity", "url_read", "allowlist", "capture",
    "scroll", "ocr", "redaction", "emit", "source_discovery", "open_readonly", "privacy_gate",
    "format_detect", "parse", "checkpoint",
}
COMPONENT_STATES = {
    "active", "install_declined", "paused_by_user", "not_installed", "pending_install",
    "awaiting_activation", "inactive_in_vscode", "bridge_offline", "unsupported_remote_host",
    "incompatible", "error",
}


def _now() -> str:
    return datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")


class AdapterHealthRegistry:
    def __init__(self, path: str | Path):
        self.path = Path(path)
        self._lock = threading.RLock()
        self._items: dict[str, dict] = {}
        self._load()

    def begin(self, adapter_id: str, stage: str) -> None:
        self._validate_adapter(adapter_id)
        self._validate_stage(stage)
        with self._lock:
            item = self._items.setdefault(adapter_id, self._default(adapter_id))
            item.update({"state": "active", "error_stage": stage, "last_scan_at": _now()})
            self._write()

    def finish(self, adapter_id: str, state: str = "active", *, discovered: int = 0, parsed: int = 0, skipped: int = 0, failed: int = 0, last_event_at: str | None = None, detected_format: str | None = None, total_files: int | None = None, covered_files: int | None = None, pending_files: int | None = None) -> None:
        self._validate_adapter(adapter_id)
        self._validate_state(state)
        with self._lock:
            item = self._items.setdefault(adapter_id, self._default(adapter_id))
            item.update({
                "state": state,
                "last_scan_at": _now(),
                "last_success_at": _now() if state in {"active", "idle"} else item.get("last_success_at"),
                "last_event_at": last_event_at or item.get("last_event_at"),
                "detected_format": detected_format[:128] if detected_format else item.get("detected_format", ""),
                "discovered": max(0, int(discovered)),
                "parsed": max(0, int(parsed)),
                "skipped": max(0, int(skipped)),
                "failed": max(0, int(failed)),
                "lag_seconds": 0,
                "error_code": "",
                "error_stage": "",
            })
            if total_files is not None:
                item["total_files"] = max(0, int(total_files))
            if covered_files is not None:
                item["covered_files"] = max(0, int(covered_files))
            if pending_files is not None:
                item["pending_files"] = max(0, int(pending_files))
            self._write()

    def fail(self, adapter_id: str, state: str, stage: str, error_code: str, **_ignored) -> None:
        self._validate_adapter(adapter_id)
        self._validate_state(state)
        self._validate_stage(stage)
        if not error_code or len(str(error_code)) > 128 or any(character in str(error_code) for character in "\\/:\n\r"):
            raise ValueError("适配器错误码无效")
        with self._lock:
            item = self._items.setdefault(adapter_id, self._default(adapter_id))
            item.update({"state": state, "last_scan_at": _now(), "error_code": str(error_code), "error_stage": stage})
            self._write()

    def snapshot(self) -> list[dict]:
        with self._lock:
            return [dict(self._items[key]) for key in sorted(self._items)]

    def update_component(self, adapter_id: str, value: dict) -> None:
        self._validate_adapter(adapter_id)
        component_state = str(value.get("component_state", ""))
        if component_state not in COMPONENT_STATES:
            raise ValueError("组件状态无效")
        state = (
            "active" if component_state == "active"
            else "disabled" if component_state in {"install_declined", "paused_by_user"}
            else "source_missing" if component_state == "not_installed"
            else "idle" if component_state in {"pending_install", "awaiting_activation"}
            else "error"
        )
        bounded_strings = {}
        for key in ("component_version", "vscode_version"):
            field = str(value.get(key, ""))
            if len(field) > 128:
                raise ValueError("组件版本字段无效")
            bounded_strings[key] = field
        counts = {}
        for key in ("pending_events", "sent_events", "dropped_events"):
            field = value.get(key, 0)
            if not isinstance(field, int) or isinstance(field, bool) or field < 0:
                raise ValueError("组件计数字段无效")
            counts[key] = field
        with self._lock:
            item = self._items.setdefault(adapter_id, self._default(adapter_id))
            item.update({
                "state": state,
                "component_state": component_state,
                "component_version": bounded_strings["component_version"],
                "protocol_version": max(0, int(value.get("protocol_version", 0))),
                "vscode_version": bounded_strings["vscode_version"],
                "last_component_heartbeat_at": value.get("last_component_heartbeat_at"),
                "pending_events": counts["pending_events"],
                "sent_events": counts["sent_events"],
                "dropped_events": counts["dropped_events"],
                "last_scan_at": _now(),
                "last_success_at": _now() if state in {"active", "idle", "disabled"} else item.get("last_success_at"),
                "error_code": component_state if state == "error" else "",
                "error_stage": "checkpoint" if state == "error" else "",
            })
            self._write()

    def _default(self, adapter_id: str) -> dict:
        return {
            "adapter_id": adapter_id,
            "state": "idle",
            "capability_version": "1",
            "detected_format": "",
            "last_scan_at": None,
            "last_success_at": None,
            "last_event_at": None,
            "discovered": 0,
            "parsed": 0,
            "skipped": 0,
            "failed": 0,
            "lag_seconds": 0,
            "error_code": "",
            "error_stage": "",
            "component_state": "",
            "component_version": "",
            "protocol_version": 0,
            "vscode_version": "",
            "last_component_heartbeat_at": None,
            "pending_events": 0,
            "sent_events": 0,
            "dropped_events": 0,
            "total_files": 0,
            "covered_files": 0,
            "pending_files": 0,
        }

    def _load(self) -> None:
        try:
            data = json.loads(self.path.read_text(encoding="utf-8"))
            if isinstance(data, list):
                self._items = {item["adapter_id"]: item for item in data if isinstance(item, dict) and item.get("adapter_id") in {str(item.get("adapter_id"))}}
        except (OSError, ValueError, TypeError, json.JSONDecodeError):
            self._items = {}

    def _write(self) -> None:
        self.path.parent.mkdir(parents=True, exist_ok=True)
        temporary = self.path.with_suffix(self.path.suffix + ".tmp")
        temporary.write_text(json.dumps(self.snapshot(), ensure_ascii=True, separators=(",", ":")), encoding="utf-8")
        temporary.replace(self.path)

    @staticmethod
    def _validate_adapter(adapter_id: str) -> None:
        if not adapter_id or len(adapter_id) > 64 or any(character not in "abcdefghijklmnopqrstuvwxyz0123456789_-" for character in adapter_id.casefold()):
            raise ValueError("适配器标识无效")

    @staticmethod
    def _validate_state(state: str) -> None:
        if state not in STATES:
            raise ValueError("适配器状态无效")

    @staticmethod
    def _validate_stage(stage: str) -> None:
        if stage not in STAGES:
            raise ValueError("适配器阶段无效")
