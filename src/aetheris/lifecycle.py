from __future__ import annotations

import os
import re
import threading
from datetime import datetime, timezone
from pathlib import Path
from typing import Callable


class LifecycleLog:
    def __init__(self, path: str | Path, max_bytes: int = 1024 * 1024, backups: int = 5):
        self.path = Path(path)
        self.max_bytes = max(1, int(max_bytes))
        self.backups = max(1, int(backups))
        self._lock = threading.RLock()

    def start(self, supervised: bool) -> None:
        self._write(f"lifecycle start supervised={'true' if supervised else 'false'} pid={os.getpid()}")

    def stop(self, reason: str) -> None:
        safe_reason = reason if reason in {"user_exit", "already_running"} else "normal_exit"
        self._write(f"lifecycle stop reason={safe_reason}")

    def fatal(self, error_type: str) -> None:
        safe_type = error_type if re.fullmatch(r"[A-Za-z][A-Za-z0-9_.]{0,127}", error_type) else "UnknownError"
        self._write(f"lifecycle fatal error={safe_type}")

    def _write(self, message: str) -> None:
        timestamp = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
        line = f"{timestamp} {message}\n"
        encoded_size = len(line.encode("utf-8"))
        with self._lock:
            self.path.parent.mkdir(parents=True, exist_ok=True)
            current_size = self.path.stat().st_size if self.path.is_file() else 0
            if current_size and current_size + encoded_size > self.max_bytes:
                self._rotate()
            with self.path.open("a", encoding="utf-8", newline="\n") as handle:
                handle.write(line)

    def _rotate(self) -> None:
        oldest = self.path.with_name(f"{self.path.name}.{self.backups}")
        oldest.unlink(missing_ok=True)
        for index in range(self.backups - 1, 0, -1):
            source = self.path.with_name(f"{self.path.name}.{index}")
            if source.is_file():
                source.replace(self.path.with_name(f"{self.path.name}.{index + 1}"))
        self.path.replace(self.path.with_name(f"{self.path.name}.1"))


def execute_core(runner: Callable[[], None], lifecycle: LifecycleLog, supervised: bool) -> int:
    lifecycle.start(supervised)
    try:
        runner()
    except Exception as exc:
        lifecycle.fatal(type(exc).__name__)
        return 1
    lifecycle.stop("user_exit")
    return 0
