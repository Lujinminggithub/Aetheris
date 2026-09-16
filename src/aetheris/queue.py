from __future__ import annotations

import sqlite3
import time
import json
from pathlib import Path

from .events import AetherisEvent


class LocalQueue:
    def __init__(self, path: str | Path, max_bytes: int = 512 * 1024 * 1024):
        self.path = Path(path)
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self.max_bytes = max_bytes
        self._connection = sqlite3.connect(self.path, timeout=10, check_same_thread=False)
        self._connection.row_factory = sqlite3.Row
        self._connection.execute("PRAGMA journal_mode=WAL")
        self._connection.execute(
            """
            CREATE TABLE IF NOT EXISTS queue_events (
                event_id TEXT PRIMARY KEY,
                event_json TEXT NOT NULL,
                status TEXT NOT NULL DEFAULT 'queued',
                attempts INTEGER NOT NULL DEFAULT 0,
                created_at REAL NOT NULL,
                claimed_at REAL,
                last_error TEXT,
                next_attempt_at REAL
            )
            """
        )
        columns = {row[1] for row in self._connection.execute("PRAGMA table_info(queue_events)").fetchall()}
        if "next_attempt_at" not in columns:
            self._connection.execute("ALTER TABLE queue_events ADD COLUMN next_attempt_at REAL")
        self._connection.execute(
            """
            CREATE TABLE IF NOT EXISTS history_events (
                event_id TEXT PRIMARY KEY,
                event_json TEXT NOT NULL,
                created_at REAL NOT NULL
            )
            """
        )
        self._connection.commit()

    def enqueue(self, event: AetherisEvent) -> None:
        payload = event.to_json()
        projected = self._connection.execute("SELECT COALESCE(SUM(LENGTH(event_json)), 0) FROM queue_events").fetchone()[0]
        if projected + len(payload.encode("utf-8")) > self.max_bytes:
            raise RuntimeError("local queue capacity exceeded")
        self._connection.execute(
            "INSERT OR IGNORE INTO queue_events(event_id, event_json, created_at) VALUES (?, ?, ?)",
            (event.event_id, payload, time.time()),
        )
        self._connection.execute(
            "INSERT OR IGNORE INTO history_events(event_id, event_json, created_at) VALUES (?, ?, ?)",
            (event.event_id, payload, time.time()),
        )
        self._connection.commit()

    def claim_batch(self, limit: int) -> list[AetherisEvent]:
        if limit <= 0:
            return []
        with self._connection:
            self._connection.execute("BEGIN IMMEDIATE")
            rows = self._connection.execute(
                "SELECT event_id, event_json FROM queue_events WHERE status = 'queued' AND (next_attempt_at IS NULL OR next_attempt_at <= ?) ORDER BY created_at LIMIT ?",
                (time.time(), limit),
            ).fetchall()
            now = time.time()
            for row in rows:
                self._connection.execute(
                    "UPDATE queue_events SET status = 'inflight', attempts = attempts + 1, claimed_at = ? WHERE event_id = ?",
                    (now, row["event_id"]),
                )
        return [AetherisEvent.from_dict(__import__("json").loads(row["event_json"])) for row in rows]

    def ack(self, event_ids: list[str]) -> None:
        if not event_ids:
            return
        self._connection.executemany("DELETE FROM queue_events WHERE event_id = ?", [(event_id,) for event_id in event_ids])
        self._connection.commit()

    def release(self, event_ids: list[str], delay_seconds: float = 0) -> None:
        if not event_ids:
            return
        self._connection.executemany(
            "UPDATE queue_events SET status = 'queued', claimed_at = NULL, next_attempt_at = ? WHERE event_id = ? AND status = 'inflight'",
            [(time.time() + max(0, delay_seconds), event_id) for event_id in event_ids],
        )
        self._connection.commit()

    def release_with_backoff(self, event_ids: list[str]) -> float:
        if not event_ids:
            return 0.0
        attempts = [
            row[0]
            for row in self._connection.execute(
                f"SELECT attempts FROM queue_events WHERE event_id IN ({','.join('?' for _ in event_ids)})",
                event_ids,
            ).fetchall()
        ]
        delay = float(min(300, 2 ** max(attempts or [1]) - 1))
        self.release(event_ids, delay_seconds=delay)
        return delay

    def reject(self, event_ids: list[str], reason: str) -> None:
        if not event_ids:
            return
        safe_reason = reason[:256]
        self._connection.executemany(
            "UPDATE queue_events SET status = 'rejected', last_error = ?, claimed_at = NULL WHERE event_id = ?",
            [(safe_reason, event_id) for event_id in event_ids],
        )
        self._connection.commit()

    def rejected_count(self) -> int:
        return int(self._connection.execute("SELECT COUNT(*) FROM queue_events WHERE status = 'rejected'").fetchone()[0])

    def history(self, limit: int = 200) -> list[dict]:
        rows = self._connection.execute(
            "SELECT event_json FROM history_events ORDER BY created_at DESC LIMIT ?",
            (max(1, min(int(limit), 1000)),),
        ).fetchall()
        return [json.loads(row[0]) for row in rows]

    def stats(self) -> dict:
        bytes_used = int(self._connection.execute("SELECT COALESCE(SUM(LENGTH(event_json)), 0) FROM queue_events").fetchone()[0])
        queued_count = int(self._connection.execute("SELECT COUNT(*) FROM queue_events WHERE status = 'queued'").fetchone()[0])
        inflight_count = int(self._connection.execute("SELECT COUNT(*) FROM queue_events WHERE status = 'inflight'").fetchone()[0])
        rejected_count = int(self._connection.execute("SELECT COUNT(*) FROM queue_events WHERE status = 'rejected'").fetchone()[0])
        ratio = bytes_used / self.max_bytes if self.max_bytes else 1.0
        watermark = "pause" if ratio >= 0.95 else "warning" if ratio >= 0.80 else "normal"
        oldest = self._connection.execute("SELECT MIN(created_at) FROM queue_events WHERE status IN ('queued', 'inflight')").fetchone()[0]
        return {
            "bytes_used": bytes_used,
            "max_bytes": self.max_bytes,
            "usage_ratio": ratio,
            "watermark": watermark,
            "queued_count": queued_count,
            "inflight_count": inflight_count,
            "rejected_count": rejected_count,
            "oldest_pending_at": oldest,
        }

    def close(self) -> None:
        self._connection.close()
