from __future__ import annotations

import ntpath
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Sequence


CORRELATION_WINDOW_SECONDS = 30
AI_SHELL_TYPES = {"shell", "powershell", "cmd", "bash"}


@dataclass(frozen=True)
class CorrelationResult:
    records: list[tuple]
    suppressed_count: int


class CommandOriginCorrelator:
    """Suppress terminal duplicates only when AI evidence is exact and recent."""

    def __init__(self):
        self._recent_ai: dict[tuple[str, str], list[datetime]] = {}

    def filter(self, records: Sequence[tuple], *, now: datetime | None = None, project_roots: Sequence[str | Path] = ()) -> CorrelationResult:
        observed_at = now or datetime.now(timezone.utc)
        cutoff = observed_at.timestamp() - CORRELATION_WINDOW_SECONDS
        self._recent_ai = {
            key: [value for value in timestamps if value.timestamp() >= cutoff]
            for key, timestamps in self._recent_ai.items()
            if any(value.timestamp() >= cutoff for value in timestamps)
        }
        for item in records:
            record = item[0] if item else {}
            if not isinstance(record, dict) or record.get("event_type") != "ai.tool_call":
                continue
            payload = record.get("payload") if isinstance(record.get("payload"), dict) else {}
            command_hash = str(payload.get("command_hash") or "").strip()
            call_type = str(payload.get("tool_call_type") or "").casefold().strip()
            timestamp = _parse_time(payload.get("timestamp")) or observed_at
            project = _project_key(payload.get("project") or payload.get("project_path") or payload.get("cwd"), project_roots)
            if not command_hash or call_type not in AI_SHELL_TYPES or not project:
                continue
            self._recent_ai.setdefault((project, command_hash), []).append(timestamp)

        kept: list[tuple] = []
        suppressed = 0
        for item in records:
            if not item:
                kept.append(item)
                continue
            record = item[0]
            if not isinstance(record, dict) or record.get("event_type") != "terminal.command":
                kept.append(item)
                continue
            payload = record.get("payload") if isinstance(record.get("payload"), dict) else {}
            command_hash = str(payload.get("command_hash") or "").strip()
            project = _project_key(item[2] if len(item) > 2 else None, project_roots)
            if not command_hash or not project:
                kept.append(item)
                continue
            terminal_time = _parse_time(payload.get("timestamp")) or observed_at
            matches = self._recent_ai.get((project, command_hash), [])
            if any(abs((terminal_time - candidate).total_seconds()) <= CORRELATION_WINDOW_SECONDS for candidate in matches):
                suppressed += 1
                continue
            kept.append(item)
        return CorrelationResult(kept, suppressed)


def _parse_time(value) -> datetime | None:
    if not isinstance(value, str) or not value.strip():
        return None
    try:
        parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError:
        return None
    return parsed.astimezone(timezone.utc) if parsed.tzinfo else None


def _project_key(value, project_roots: Sequence[str | Path] = ()) -> str:
    if value is None:
        return ""
    raw = str(value).strip().replace("/", "\\")
    if not raw:
        return ""
    normalized = ntpath.normcase(ntpath.normpath(raw))
    roots = sorted((_project_key(item) for item in project_roots), key=len, reverse=True)
    for root in roots:
        try:
            if ntpath.commonpath([normalized, root]) == root:
                return root
        except ValueError:
            continue
    return normalized
