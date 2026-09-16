from __future__ import annotations

import json
import sqlite3
from pathlib import Path

from ..redaction import Redactor


class CursorSessionAdapter:
    """Read Cursor composer bubble values from cursorDiskKV without touching auth keys."""

    tool = "cursor"

    def __init__(self, database: str | Path, max_records: int = 500):
        self.database = Path(database).expanduser().resolve()
        self.max_records = max_records
        self._seen: set[str] = set()
        self._redactor = Redactor()

    def collect(self) -> list[dict]:
        if not self.database.is_file():
            return []
        try:
            con = sqlite3.connect(f"file:{self.database}?mode=ro", uri=True, timeout=0.5)
            rows = con.execute("SELECT key, value FROM cursorDiskKV WHERE key LIKE 'bubbleId:%' ORDER BY key").fetchall()
            con.close()
        except (sqlite3.Error, OSError):
            return []
        records = []
        for key, raw_value in rows:
            if not key or key in self._seen:
                continue
            try:
                value = json.loads(raw_value) if isinstance(raw_value, str) else {}
            except (TypeError, json.JSONDecodeError):
                continue
            if not isinstance(value, dict):
                continue
            role = value.get("role") or value.get("type")
            if isinstance(role, int):
                role = {1: "user", 2: "assistant"}.get(role)
            content = value.get("content") or value.get("text") or value.get("message")
            if role not in {"user", "assistant", "tool", "system"} or not isinstance(content, str):
                continue
            self._seen.add(key)
            safe, report = self._redactor.redact(content)
            key_parts = key.split(":", 2)
            composer_id = value.get("composerId") or (key_parts[1] if len(key_parts) > 2 else None)
            bubble_id = value.get("bubbleId") or (key_parts[2] if len(key_parts) > 2 else None)
            context = value.get("context")
            context_paths = [item.get("path") for item in context if isinstance(item, dict) and item.get("path")] if isinstance(context, list) else []
            tool_calls = [item.get("name") for item in value.get("toolCalls", []) if isinstance(item, dict) and item.get("name")]
            tool_former = value.get("toolFormerData")
            if isinstance(tool_former, dict) and tool_former.get("name"):
                tool_calls.append(tool_former["name"])
            model_info = value.get("modelInfo") if isinstance(value.get("modelInfo"), dict) else {}
            records.append({
                "event_type": "ai.message",
                "payload": {
                    "tool": "cursor", "role": role, "content": safe,
                    "composer_id": composer_id, "bubble_id": bubble_id,
                    "model": value.get("model") or model_info.get("modelName") or model_info.get("name"),
                    "context_paths": context_paths, "tool_calls": list(dict.fromkeys(tool_calls)),
                    "status": value.get("status"), "code_block_count": len(value.get("codeBlocks", [])) if isinstance(value.get("codeBlocks"), list) else 0,
                },
                "redaction_report": report,
                "provenance": {"source_file": self.database.name, "source_key": key},
            })
            if len(records) >= self.max_records:
                return records
        return records

    def reset_history(self) -> None:
        self._seen.clear()
