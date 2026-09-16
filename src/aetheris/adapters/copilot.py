from __future__ import annotations

import sqlite3
from pathlib import Path

from ..redaction import Redactor


class CopilotSessionAdapter:
    """Read GitHub Copilot Chat's structured session-store SQLite tables read-only."""

    tool = "github_copilot"

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
            sessions = {row[0]: row for row in con.execute("SELECT id, cwd, repository, branch FROM sessions").fetchall()}
            rows = con.execute("SELECT id, session_id, turn_index, user_message, assistant_response, timestamp FROM turns ORDER BY timestamp, turn_index").fetchall()
            con.close()
        except (sqlite3.Error, OSError):
            return []
        records = []
        for turn_id, session_id, turn_index, user_message, assistant_response, timestamp in rows:
            for role, content in (("user", user_message), ("assistant", assistant_response)):
                if not content:
                    continue
                key = f"{turn_id}:{role}"
                if key in self._seen:
                    continue
                self._seen.add(key)
                safe, report = self._redactor.redact(str(content))
                session = sessions.get(session_id) or (session_id, "", "", "")
                records.append({
                    "event_type": "ai.message",
                    "payload": {"tool": "github_copilot", "role": role, "content": safe, "session_id": session_id, "turn_index": turn_index, "timestamp": timestamp, "project_path": session[1], "repository": session[2], "branch": session[3]},
                    "redaction_report": report,
                    "provenance": {"source_file": self.database.name, "turn_id": turn_id, "role": role},
                })
                if len(records) >= self.max_records:
                    return records
        return records

    def reset_history(self) -> None:
        self._seen.clear()
