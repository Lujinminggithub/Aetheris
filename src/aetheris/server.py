from __future__ import annotations

import json
import hashlib
import hmac
import os
import secrets
import sqlite3
import time
from collections import Counter
from pathlib import Path

from .events import AetherisEvent
from .forge import Forge
from .pulse import Pulse
from .logs import DailyLog
from .dataset import DatasetExporter


class EventStore:
    def __init__(self, path: str | Path):
        self.path = Path(path)
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self.connection = sqlite3.connect(self.path, timeout=10, check_same_thread=False)
        self.connection.row_factory = sqlite3.Row
        self.connection.execute(
            """
            CREATE TABLE IF NOT EXISTS events (
                event_id TEXT PRIMARY KEY,
                event_type TEXT NOT NULL,
                occurred_at TEXT NOT NULL,
                content_hash TEXT NOT NULL,
                event_json TEXT NOT NULL,
                ingested_at REAL NOT NULL
            )
            """
        )
        self.connection.execute(
            "CREATE TABLE IF NOT EXISTS event_tombstones (event_id TEXT PRIMARY KEY, reason TEXT NOT NULL, deleted_at REAL NOT NULL)"
        )
        self.connection.execute(
            """
            CREATE TABLE IF NOT EXISTS devices (
                device_id TEXT PRIMARY KEY,
                client_version TEXT NOT NULL,
                hostname TEXT,
                platform TEXT,
                status TEXT NOT NULL DEFAULT 'online',
                registered_at REAL NOT NULL,
                last_seen REAL NOT NULL,
                metadata_json TEXT NOT NULL
            )
            """
        )
        self.connection.execute(
            """
            CREATE TABLE IF NOT EXISTS admin_users (
                username TEXT PRIMARY KEY,
                password_salt TEXT NOT NULL,
                password_hash TEXT NOT NULL,
                must_change INTEGER NOT NULL DEFAULT 1,
                created_at REAL NOT NULL,
                updated_at REAL NOT NULL
            )
            """
        )
        self.connection.commit()

    @staticmethod
    def _password_digest(password: str, salt: bytes) -> str:
        return hashlib.pbkdf2_hmac("sha256", password.encode("utf-8"), salt, 240_000).hex()

    def ensure_admin(self, bootstrap_password: str | None, username: str = "admin") -> None:
        if self.connection.execute("SELECT 1 FROM admin_users WHERE username = ?", (username,)).fetchone():
            return
        if not bootstrap_password:
            return
        salt = os.urandom(16)
        now = time.time()
        self.connection.execute(
            "INSERT INTO admin_users(username, password_salt, password_hash, must_change, created_at, updated_at) VALUES (?, ?, ?, 1, ?, ?)",
            (username, salt.hex(), self._password_digest(bootstrap_password, salt), now, now),
        )
        self.connection.commit()

    def authenticate_admin(self, username: str, password: str) -> dict | None:
        row = self.connection.execute("SELECT * FROM admin_users WHERE username = ?", (username,)).fetchone()
        if not row:
            return None
        expected = self._password_digest(password, bytes.fromhex(row["password_salt"]))
        if not hmac.compare_digest(expected, row["password_hash"]):
            return None
        return {"username": row["username"], "must_change": bool(row["must_change"])}

    def change_admin_password(self, username: str, old_password: str, new_password: str) -> bool:
        if len(new_password) < 10 or self.authenticate_admin(username, old_password) is None:
            return False
        salt = os.urandom(16)
        self.connection.execute(
            "UPDATE admin_users SET password_salt = ?, password_hash = ?, must_change = 0, updated_at = ? WHERE username = ?",
            (salt.hex(), self._password_digest(new_password, salt), time.time(), username),
        )
        self.connection.commit()
        return True

    def insert_if_new(self, event: AetherisEvent) -> str:
        if self.connection.execute("SELECT 1 FROM event_tombstones WHERE event_id = ?", (event.event_id,)).fetchone():
            return "tombstoned"
        cursor = self.connection.execute(
            """
            INSERT OR IGNORE INTO events(event_id, event_type, occurred_at, content_hash, event_json, ingested_at)
            VALUES (?, ?, ?, ?, ?, ?)
            """,
            (event.event_id, event.event_type, event.occurred_at, event.content_hash, event.to_json(), time.time()),
        )
        self.connection.commit()
        return "accepted" if cursor.rowcount == 1 else "duplicate"

    def delete_event(self, event_id: str, reason: str) -> None:
        self.connection.execute("DELETE FROM events WHERE event_id = ?", (event_id,))
        self.connection.execute(
            "INSERT OR REPLACE INTO event_tombstones(event_id, reason, deleted_at) VALUES (?, ?, ?)",
            (event_id, reason[:256], time.time()),
        )
        self.connection.commit()

    def count(self) -> int:
        return int(self.connection.execute("SELECT COUNT(*) FROM events").fetchone()[0])

    def register_device(self, device_id: str, *, client_version: str, hostname: str = "", platform: str = "", metadata: dict | None = None) -> dict:
        if not device_id or not client_version:
            raise ValueError("device_id and client_version are required")
        now = time.time()
        self.connection.execute(
            """
            INSERT INTO devices(device_id, client_version, hostname, platform, status, registered_at, last_seen, metadata_json)
            VALUES (?, ?, ?, ?, 'online', ?, ?, ?)
            ON CONFLICT(device_id) DO UPDATE SET client_version=excluded.client_version,
                hostname=excluded.hostname, platform=excluded.platform, status='online', last_seen=excluded.last_seen,
                metadata_json=excluded.metadata_json
            """,
            (device_id, client_version, hostname[:255], platform[:100], now, now, json.dumps(metadata or {}, ensure_ascii=True, sort_keys=True)),
        )
        self.connection.commit()
        return {"device_id": device_id, "status": "registered", "last_seen": now}

    def list_devices(self) -> list[dict]:
        rows = self.connection.execute("SELECT device_id, client_version, hostname, platform, status, registered_at, last_seen FROM devices ORDER BY last_seen DESC").fetchall()
        return [dict(row) for row in rows]

    def fetch_all(self) -> list[dict[str, str]]:
        return [dict(row) for row in self.connection.execute("SELECT * FROM events ORDER BY ingested_at").fetchall()]

    def list_events(self, limit: int = 100, *, project_id: str | None = None, event_type: str | None = None) -> list[dict]:
        limit = max(1, min(int(limit), 1000))
        clauses = []
        values: list[object] = []
        if project_id:
            clauses.append("json_extract(event_json, '$.project_id') = ?")
            values.append(project_id)
        if event_type:
            clauses.append("event_type = ?")
            values.append(event_type)
        where = f"WHERE {' AND '.join(clauses)}" if clauses else ""
        rows = self.connection.execute(
            f"SELECT event_json FROM events {where} ORDER BY occurred_at DESC LIMIT ?",
            (*values, limit),
        ).fetchall()
        return [json.loads(row["event_json"]) for row in rows]

    def summary(self) -> dict:
        events = self.list_events(limit=1000)
        by_type = Counter(event.get("event_type", "unknown") for event in events)
        by_project = Counter(event.get("project_id", "unknown") for event in events)
        return {
            "total_events": len(events),
            "by_event_type": dict(sorted(by_type.items())),
            "by_project": dict(sorted(by_project.items())),
            "latest_occurred_at": events[0].get("occurred_at") if events else None,
        }

    def export(self, format_name: str) -> str:
        events = self.list_events(limit=1000)
        if format_name == "jsonl":
            return "".join(json.dumps(event, ensure_ascii=True, sort_keys=True, separators=(",", ":")) + "\n" for event in events)
        if format_name == "markdown":
            lines = ["# Aetheris Events", "", f"Total events: {len(events)}", ""]
            for event in events:
                lines.append(f"- `{event.get('occurred_at')}` **{event.get('event_type')}** project `{event.get('project_id')}`")
                lines.append(f"  - event_id: `{event.get('event_id')}`")
                lines.append(f"  - payload: `{json.dumps(event.get('payload', {}), ensure_ascii=True, sort_keys=True)}`")
            return "\n".join(lines) + "\n"
        if format_name == "openai_messages":
            rows = []
            for event in events:
                content = event.get("payload", {}).get("content") or json.dumps(event.get("payload", {}), ensure_ascii=True, sort_keys=True)
                rows.append(json.dumps({"event_id": event.get("event_id"), "messages": [{"role": "user", "content": content}]}, ensure_ascii=True, separators=(",", ":")))
            return "\n".join(rows) + ("\n" if rows else "")
        if format_name == "agent_trajectory":
            rows = []
            for event in events:
                content = json.dumps(event.get("payload", {}), ensure_ascii=True, sort_keys=True)
                rows.append(json.dumps({"event_id": event.get("event_id"), "trajectory": [{"role": "tool", "content": content}]}, ensure_ascii=True, separators=(",", ":")))
            return "\n".join(rows) + ("\n" if rows else "")
        raise ValueError(f"unsupported export format: {format_name}")

    def episodes(self) -> list[dict]:
        events = [AetherisEvent.from_dict(item) for item in self.list_events(limit=1000)]
        return Forge().build(events)

    def pulse(self) -> dict:
        return Pulse().summarize(AetherisEvent.from_dict(item) for item in self.list_events(limit=1000))

    def daily_log(self) -> str:
        return DailyLog().build(AetherisEvent.from_dict(item) for item in self.list_events(limit=1000))

    def dataset(self) -> dict:
        return DatasetExporter().export(AetherisEvent.from_dict(item) for item in self.list_events(limit=1000))

    def close(self) -> None:
        self.connection.close()


def serialize_response(payload: dict, status: int = 200) -> tuple[int, bytes]:
    return status, json.dumps(payload, ensure_ascii=True, separators=(",", ":")).encode("utf-8")
