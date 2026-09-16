from __future__ import annotations

import ctypes
import json
import os
import re
from ctypes import wintypes
from dataclasses import dataclass
from pathlib import Path
from typing import Callable

from ..processes import NativeProcessIdentityProvider
from ..redaction import Redactor


@dataclass(frozen=True)
class VSCodeWindow:
    pid: int
    title: str
    hwnd: int = 0


def enumerate_vscode_windows() -> list[VSCodeWindow]:
    if os.name != "nt":
        return []
    user32 = ctypes.windll.user32
    identity = NativeProcessIdentityProvider()
    windows: list[VSCodeWindow] = []
    callback_type = ctypes.WINFUNCTYPE(wintypes.BOOL, wintypes.HWND, wintypes.LPARAM)

    @callback_type
    def callback(hwnd, _lparam):
        if not user32.IsWindowVisible(hwnd):
            return True
        length = user32.GetWindowTextLengthW(hwnd)
        if length <= 0:
            return True
        title_buffer = ctypes.create_unicode_buffer(length + 1)
        user32.GetWindowTextW(hwnd, title_buffer, length + 1)
        title = title_buffer.value.strip()
        if "visual studio code" not in title.casefold():
            return True
        pid = wintypes.DWORD()
        user32.GetWindowThreadProcessId(hwnd, ctypes.byref(pid))
        executable = identity.process_path(int(pid.value)) or ""
        if Path(executable).name.casefold() == "code.exe":
            windows.append(VSCodeWindow(int(pid.value), title, int(hwnd)))
        return True

    user32.EnumWindows(callback, 0)
    return windows


class VSCodeAdapter:
    """Collect VS Code window metadata without reading source file contents."""

    def __init__(
        self,
        state_path: str | Path | None = None,
        *,
        project_roots: list[str | Path] | None = None,
        window_provider: Callable[[], list[VSCodeWindow]] | None = None,
    ):
        self.state_path = Path(state_path) if state_path else self.default_state_path()
        self.project_roots = [Path(root).expanduser().resolve() for root in (project_roots or [])]
        self.window_provider = window_provider or enumerate_vscode_windows
        self._legacy_bridge = state_path is not None and self.state_path.suffix.casefold() == ".json"
        self._redactor = Redactor()
        self._last_fingerprint = ""
        self._window_fingerprints: dict[int, str] = {}
        self.last_discovered = 0

    @staticmethod
    def default_state_path() -> Path:
        appdata = os.environ.get("APPDATA")
        if appdata:
            return Path(appdata) / "Code" / "User" / "globalStorage" / "state.vscdb"
        return Path.home() / ".config" / "Code" / "User" / "globalStorage" / "state.vscdb"

    @property
    def detected_format(self) -> str:
        return "native_window+state.vscdb" if self.state_path.is_file() else "native_window"

    def update_project_roots(self, roots: list[str | Path]) -> None:
        self.project_roots = [Path(root).expanduser().resolve() for root in roots]

    def collect(self) -> list[dict]:
        if self._legacy_bridge:
            return self._collect_legacy_bridge()
        windows = self.window_provider()
        self.last_discovered = len(windows)
        current: dict[int, str] = {}
        records: list[dict] = []
        for window in windows:
            parsed = _parse_window_title(window.title)
            if parsed is None:
                continue
            fingerprint = json.dumps(parsed, sort_keys=True, ensure_ascii=True)
            current[window.pid] = fingerprint
            if self._window_fingerprints.get(window.pid) == fingerprint:
                continue
            safe, report = self._redactor.redact({"tool": "vscode", **parsed})
            record = {
                "event_type": "ide.activity",
                "payload": safe,
                "redaction_report": report,
                "provenance": {"capture_method": "native_window", "process_name": "Code.exe"},
            }
            project_path = self._project_path(parsed["workspace"])
            if project_path is not None:
                record["project_path"] = str(project_path)
            records.append(record)
        self._window_fingerprints = current
        return records

    def _project_path(self, workspace: str) -> Path | None:
        matches = [root for root in self.project_roots if root.name.casefold() == workspace.casefold()]
        if not matches:
            return None
        matches.sort(key=lambda root: (".worktrees" in {part.casefold() for part in root.parts}, len(root.parts), str(root).casefold()))
        return matches[0]

    def _collect_legacy_bridge(self) -> list[dict]:
        self.last_discovered = 1 if self.state_path.is_file() else 0
        if not self.state_path.is_file():
            return []
        try:
            data = json.loads(self.state_path.read_text(encoding="utf-8-sig"))
        except (OSError, json.JSONDecodeError):
            return []
        safe, report = self._redactor.redact({
            "workspace": data.get("workspace", ""),
            "active_file": data.get("active_file", ""),
            "last_command": data.get("last_command", ""),
        })
        fingerprint = json.dumps(safe, sort_keys=True, ensure_ascii=True)
        if fingerprint == self._last_fingerprint:
            return []
        self._last_fingerprint = fingerprint
        return [{"event_type": "ide.activity", "payload": safe, "redaction_report": report}]


def _parse_window_title(title: str) -> dict | None:
    value = re.sub(r"\s+-\s+Visual Studio Code(?:\s+-\s+Insiders)?$", "", title.strip(), flags=re.IGNORECASE)
    if not value or value == title.strip():
        return None
    parts = [part.strip() for part in value.split(" - ") if part.strip()]
    if not parts:
        return None
    active_file = parts[0].lstrip("● ").strip() if len(parts) > 1 else ""
    workspace = parts[-1]
    extension = Path(active_file).suffix.casefold()[:32] if active_file else ""
    return {"workspace": workspace[:255], "active_file": active_file[:255], "active_file_extension": extension}
