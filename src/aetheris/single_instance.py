from __future__ import annotations

import ctypes
import hashlib
import json
import os
from pathlib import Path


def mutex_name(user_sid: str) -> str:
    digest = hashlib.sha256(user_sid.strip().casefold().encode("utf-8")).hexdigest()[:16]
    return f"Local\\AetherisCore-{digest}"


class SingleInstance:
    def __init__(self, user_key: str | None = None):
        self.user_key = user_key or os.environ.get("USERNAME", "current-user")
        self.handle = None

    def acquire(self) -> bool:
        if os.name != "nt":
            return True
        self.handle = ctypes.windll.kernel32.CreateMutexW(None, False, mutex_name(self.user_key))
        if not self.handle:
            raise ctypes.WinError()
        return ctypes.windll.kernel32.GetLastError() != 183

    def close(self) -> None:
        if self.handle:
            ctypes.windll.kernel32.CloseHandle(self.handle)
            self.handle = None


def existing_status_url(config_path: str | Path) -> str | None:
    try:
        config = json.loads(Path(config_path).read_text(encoding="utf-8-sig"))
        queue = Path(config["queue"]).expanduser().resolve()
        status = json.loads((queue.parent / "core-status.json").read_text(encoding="utf-8"))
        url = str(status.get("local_view_url", ""))
        return url if url.startswith("http://127.0.0.1:") else None
    except (OSError, KeyError, ValueError, json.JSONDecodeError):
        return None
