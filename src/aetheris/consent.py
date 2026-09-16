from __future__ import annotations

import sqlite3
import time
from dataclasses import dataclass
from pathlib import Path


@dataclass(frozen=True)
class ProcessIdentity:
    name: str
    executable: str
    publisher: str
    binary_hash: str


@dataclass(frozen=True)
class ConsentGrant:
    identity: ProcessIdentity
    scope: str
    project_id: str
    decision: str


class ConsentStore:
    def __init__(self, path: str | Path):
        self.connection = sqlite3.connect(Path(path), check_same_thread=False)
        self.connection.execute(
            """
            CREATE TABLE IF NOT EXISTS consent_grants (
                name TEXT NOT NULL,
                executable TEXT NOT NULL,
                publisher TEXT NOT NULL,
                binary_hash TEXT NOT NULL,
                scope TEXT NOT NULL,
                project_id TEXT NOT NULL,
                decision TEXT NOT NULL,
                created_at REAL NOT NULL,
                PRIMARY KEY(name, executable, publisher, binary_hash, scope, project_id)
            )
            """
        )
        self.connection.commit()

    def save(self, grant: ConsentGrant) -> None:
        self.connection.execute(
            "INSERT OR REPLACE INTO consent_grants VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
            (
                grant.identity.name.casefold(),
                grant.identity.executable,
                grant.identity.publisher,
                grant.identity.binary_hash,
                grant.scope,
                grant.project_id,
                grant.decision,
                time.time(),
            ),
        )
        self.connection.commit()

    def decide(self, identity: ProcessIdentity, *, project_id: str) -> str:
        rows = self.connection.execute(
            "SELECT scope, project_id, decision FROM consent_grants WHERE name = ? AND executable = ? AND publisher = ? AND binary_hash = ?",
            (identity.name.casefold(), identity.executable, identity.publisher, identity.binary_hash),
        ).fetchall()
        for scope, granted_project, decision in rows:
            if scope == "global" or (scope == "project" and granted_project == project_id):
                return "excluded" if decision in {"deny", "always_ignore"} else "allowed"
        return "pending"

    def close(self) -> None:
        self.connection.close()

