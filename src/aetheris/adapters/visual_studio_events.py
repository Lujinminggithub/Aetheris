from __future__ import annotations

import json
from pathlib import Path

from ..redaction import Redactor


class VisualStudioEventAdapter:
    """Consume an explicit Visual Studio activity bridge JSONL file."""

    _types = {"debug": "ide.debug", "build": "ide.build", "test": "ide.test"}

    def __init__(self, log_path: str | Path):
        self.log_path = Path(log_path).expanduser().resolve()
        self.offset = 0
        self.redactor = Redactor()

    def collect(self) -> list[dict]:
        if not self.log_path.is_file():
            return []
        try:
            with self.log_path.open("r", encoding="utf-8-sig", errors="replace") as handle:
                handle.seek(self.offset)
                lines = handle.readlines()
                self.offset = handle.tell()
        except OSError:
            return []
        records = []
        for line in lines:
            try:
                item = json.loads(line)
            except json.JSONDecodeError:
                continue
            event_type = self._types.get(item.get("event"))
            if not event_type:
                continue
            safe, report = self.redactor.redact({key: value for key, value in item.items() if key != "event"})
            records.append({"event_type": event_type, "payload": safe, "redaction_report": report})
        return records

