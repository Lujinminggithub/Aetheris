from __future__ import annotations

from pathlib import Path

from ..redaction import Redactor


class VisualStudioAdapter:
    """Read solution identity and file metadata, never source contents."""

    def __init__(self, solution_path: str | Path):
        self.solution_path = Path(solution_path).expanduser().resolve()
        self._last_fingerprint = ""
        self._redactor = Redactor()

    def collect(self) -> list[dict]:
        if not self.solution_path.is_file():
            return []
        stat = self.solution_path.stat()
        fingerprint = f"{self.solution_path}:{stat.st_mtime_ns}:{stat.st_size}"
        if fingerprint == self._last_fingerprint:
            return []
        self._last_fingerprint = fingerprint
        safe_path, report = self._redactor.redact(str(self.solution_path))
        return [{
            "event_type": "ide.activity",
            "payload": {"tool": "visual_studio", "solution_name": self.solution_path.name, "solution_path": safe_path, "size_bytes": stat.st_size},
            "redaction_report": report,
        }]

