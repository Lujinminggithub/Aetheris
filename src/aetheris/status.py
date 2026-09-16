from __future__ import annotations

import json
import threading
from datetime import datetime, timezone
from pathlib import Path


def _now() -> str:
    return datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")


class HeartbeatBackoff:
    def __init__(self):
        self._next = 5

    def failure(self) -> int:
        value = self._next
        self._next = min(300, self._next * 2)
        return value

    def success(self) -> int:
        self._next = 5
        return 60


class CoreStatus:
    def __init__(self, path: str | Path, *, version: str, device_id: str = "", gateway_url: str = ""):
        self.path = Path(path)
        self._lock = threading.RLock()
        self._data = {
            "version": version,
            "device_id": device_id,
            "gateway_url": gateway_url,
            "registration": {"state": "pending", "last_heartbeat_at": None, "last_error": ""},
            "capture": {"state": "starting", "project_count": 0, "last_capture_at": None, "last_error": "", "suppressed_duplicate_terminal": 0},
            "browser_policy": {"state": "disabled", "revision": 0, "domain_count": 0, "last_error": ""},
            "project_identity": {"state": "pending", "key_version": 0, "last_error": ""},
            "project_sync": {"state": "pending", "registry_revision": 0, "project_count": 0, "last_error": ""},
            "queue": {"queued": 0, "retrying": 0, "rejected": 0},
            "updated_at": _now(),
        }
        self._write()

    def update_registration(self, state: str, *, error: str = "", heartbeat_at: str | None = None) -> None:
        with self._lock:
            self._data["registration"] = {
                "state": state,
                "last_heartbeat_at": heartbeat_at if heartbeat_at is not None else self._data["registration"].get("last_heartbeat_at"),
                "last_error": error[:512],
            }
            self._write()

    def update_capture(self, state: str, *, project_count: int, error: str = "", capture_at: str | None = None, suppressed_duplicate_terminal: int | None = None) -> None:
        with self._lock:
            self._data["capture"] = {
                "state": state,
                "project_count": project_count,
                "last_capture_at": capture_at if capture_at is not None else self._data["capture"].get("last_capture_at"),
                "last_error": error[:512],
                "suppressed_duplicate_terminal": max(0, int(suppressed_duplicate_terminal if suppressed_duplicate_terminal is not None else self._data["capture"].get("suppressed_duplicate_terminal", 0))),
            }
            self._write()

    def update_queue(self, *, queued: int, retrying: int, rejected: int) -> None:
        with self._lock:
            self._data["queue"] = {"queued": queued, "retrying": retrying, "rejected": rejected}
            self._write()

    def update_browser_policy(self, *, state: str, revision: int, domain_count: int, error: str = "") -> None:
        with self._lock:
            self._data["browser_policy"] = {
                "state": state,
                "revision": max(0, int(revision)),
                "domain_count": max(0, int(domain_count)),
                "last_error": error[:512],
            }
            self._write()

    def update_project_identity(self, state: str, *, key_version: int = 0, error: str = "") -> None:
        with self._lock:
            self._data["project_identity"] = {
                "state": state,
                "key_version": max(0, int(key_version)),
                "last_error": error[:128],
            }
            self._write()

    def update_project_sync(self, state: str, *, registry_revision: int = 0, project_count: int = 0, error: str = "") -> None:
        with self._lock:
            self._data["project_sync"] = {
                "state": state,
                "registry_revision": max(0, int(registry_revision)),
                "project_count": max(0, int(project_count)),
                "last_error": error[:128],
            }
            self._write()

    def snapshot(self) -> dict:
        with self._lock:
            return json.loads(json.dumps(self._data))

    def set_local_view(self, url: str) -> None:
        with self._lock:
            self._data["local_view_url"] = url
            self._write()

    def _write(self) -> None:
        self._data["updated_at"] = _now()
        self.path.parent.mkdir(parents=True, exist_ok=True)
        temporary = self.path.with_suffix(self.path.suffix + ".tmp")
        temporary.write_text(json.dumps(self._data, ensure_ascii=True, indent=2), encoding="utf-8")
        temporary.replace(self.path)
