from __future__ import annotations

import hashlib
import ntpath
import sqlite3
from dataclasses import dataclass
from pathlib import Path


DEFAULT_EXCLUDED = (
    "system", "smss.exe", "csrss.exe", "winlogon.exe", "spoolsv.exe",
    "securityhealth", "defender", "teams", "slack", "wechat", "qq.exe",
    "lsass.exe", "credential", "keepass", "1password",
)


@dataclass(frozen=True)
class StableProcessIdentity:
    name: str
    executable: str
    publisher: str
    binary_hash: str
    signature_status: str = "unknown"

    @property
    def key(self) -> str:
        raw = "\0".join((_normalize_name(self.name), _normalize_executable(self.executable)))
        return hashlib.sha256(raw.encode("utf-8")).hexdigest()


def _normalize_name(name: str) -> str:
    return str(name or "").strip().casefold()


def _normalize_executable(executable: str) -> str:
    value = str(executable or "").strip().strip('"')
    if not value:
        return ""
    if value.startswith("\\\\?\\"):
        value = value[4:]
    return ntpath.normcase(ntpath.normpath(value.replace("/", "\\"))).casefold()


@dataclass(frozen=True)
class ConsentOutcome:
    state: str
    should_notify: bool
    identity_key: str
    project_ids: tuple[str, ...] = ()


def can_capture(outcome: ConsentOutcome, logical_project_id: str) -> bool:
    if outcome.state == "allow_global":
        return True
    return outcome.state == "allow_project" and logical_project_id in outcome.project_ids


class ProcessConsentCoordinator:
    def __init__(self, path: str | Path, *, user_sid: str, excluded_fragments: tuple[str, ...] = DEFAULT_EXCLUDED):
        if not user_sid:
            raise ValueError("Windows 用户 SID 不能为空")
        self.path = Path(path)
        self.user_sid = user_sid
        self.excluded_fragments = tuple(item.casefold() for item in excluded_fragments)
        self.connection = sqlite3.connect(self.path, check_same_thread=False)
        self.connection.execute("""
            CREATE TABLE IF NOT EXISTS process_consents (
                user_sid TEXT NOT NULL,
                identity_key TEXT NOT NULL,
                name TEXT NOT NULL,
                executable TEXT NOT NULL,
                publisher TEXT NOT NULL,
                binary_hash TEXT NOT NULL,
                state TEXT NOT NULL,
                project_ids TEXT NOT NULL DEFAULT '',
                first_seen REAL NOT NULL DEFAULT (strftime('%s','now')),
                last_seen REAL NOT NULL DEFAULT (strftime('%s','now')),
                prompt_shown INTEGER NOT NULL DEFAULT 0,
                decided_at REAL,
                PRIMARY KEY(user_sid, identity_key)
            )
        """)
        self.connection.commit()
        self._migrate_name_path_identities()

    def _migrate_name_path_identities(self) -> None:
        rows = self.connection.execute(
            "SELECT identity_key,name,executable,publisher,binary_hash,state,project_ids,first_seen,last_seen,prompt_shown,decided_at "
            "FROM process_consents WHERE user_sid=?",
            (self.user_sid,),
        ).fetchall()
        if not rows:
            return
        groups: dict[str, list[tuple]] = {}
        for row in rows:
            key = StableProcessIdentity(row[1], row[2], row[3], row[4]).key
            groups.setdefault(key, []).append(row)
        if len(groups) == len(rows) and all(row[0] == key for key, values in groups.items() for row in values):
            return

        merged = []
        for key, values in groups.items():
            metadata = max(values, key=lambda row: (float(row[8]), float(row[7]), row[0]))
            decided = [row for row in values if row[5] != "pending"]
            decision = max(decided, key=lambda row: (float(row[10] or 0), float(row[8]), row[0])) if decided else metadata
            state = decision[5] if decided else "pending"
            project_ids = decision[6] if state == "allow_project" else ""
            merged.append((
                self.user_sid, key, metadata[1], metadata[2], metadata[3], metadata[4], state, project_ids,
                min(float(row[7]) for row in values), max(float(row[8]) for row in values),
                max(int(row[9]) for row in values), decision[10] if decided else None,
            ))

        with self.connection:
            self.connection.execute("DELETE FROM process_consents WHERE user_sid=?", (self.user_sid,))
            self.connection.executemany(
                "INSERT INTO process_consents(user_sid,identity_key,name,executable,publisher,binary_hash,state,project_ids,first_seen,last_seen,prompt_shown,decided_at) "
                "VALUES(?,?,?,?,?,?,?,?,?,?,?,?)",
                merged,
            )

    def observe(self, identity: StableProcessIdentity) -> ConsentOutcome:
        if self._excluded(identity):
            return ConsentOutcome("default_excluded", False, identity.key)
        row = self.connection.execute(
            "SELECT state,project_ids,prompt_shown FROM process_consents WHERE user_sid=? AND identity_key=?",
            (self.user_sid, identity.key),
        ).fetchone()
        if row is None:
            self.connection.execute(
                "INSERT INTO process_consents(user_sid,identity_key,name,executable,publisher,binary_hash,state,prompt_shown) VALUES(?,?,?,?,?,?,?,1)",
                (self.user_sid, identity.key, identity.name, identity.executable, identity.publisher, identity.binary_hash, "pending"),
            )
            self.connection.commit()
            return ConsentOutcome("pending", True, identity.key)
        self.connection.execute(
            "UPDATE process_consents SET name=?,executable=?,publisher=?,binary_hash=?,last_seen=strftime('%s','now') WHERE user_sid=? AND identity_key=?",
            (identity.name, identity.executable, identity.publisher, identity.binary_hash, self.user_sid, identity.key),
        )
        self.connection.commit()
        projects = tuple(item for item in row[1].split(",") if item)
        return ConsentOutcome(row[0], False, identity.key, projects)

    def decide(self, identity: StableProcessIdentity, state: str, project_ids: list[str] | tuple[str, ...]) -> ConsentOutcome:
        if state not in {"allow_global", "allow_project", "deny", "always_ignore"}:
            raise ValueError("进程授权决定无效")
        if state == "allow_project" and not project_ids:
            raise ValueError("项目范围授权必须指定项目")
        observed = self.observe(identity)
        serialized_projects = ",".join(sorted({str(item) for item in project_ids if str(item)}))
        self.connection.execute(
            "UPDATE process_consents SET state=?,project_ids=?,decided_at=strftime('%s','now'),last_seen=strftime('%s','now') WHERE user_sid=? AND identity_key=?",
            (state, serialized_projects, self.user_sid, identity.key),
        )
        self.connection.commit()
        return ConsentOutcome(state, False, observed.identity_key, tuple(item for item in serialized_projects.split(",") if item))

    def pending(self) -> list[dict]:
        rows = self.connection.execute(
            "SELECT identity_key,name,executable,publisher,binary_hash,first_seen,last_seen FROM process_consents WHERE user_sid=? AND state='pending' ORDER BY first_seen,identity_key",
            (self.user_sid,),
        ).fetchall()
        return [{"identity_key": row[0], "name": row[1], "executable": row[2], "publisher": row[3], "binary_hash": row[4], "first_seen": row[5], "last_seen": row[6]} for row in rows]

    def list_all(self) -> list[dict]:
        rows = self.connection.execute(
            "SELECT identity_key,name,executable,publisher,binary_hash,state,project_ids,first_seen,last_seen,decided_at FROM process_consents WHERE user_sid=? "
            "ORDER BY CASE WHEN state='pending' THEN 0 ELSE 1 END,last_seen DESC,identity_key",
            (self.user_sid,),
        ).fetchall()
        return [{
            "identity_key": row[0], "name": row[1], "executable": row[2], "publisher": row[3],
            "binary_hash": row[4], "state": row[5], "project_ids": [item for item in row[6].split(",") if item],
            "first_seen": row[7], "last_seen": row[8], "decided_at": row[9],
        } for row in rows]

    def reset(self, identity: StableProcessIdentity) -> None:
        self.connection.execute("DELETE FROM process_consents WHERE user_sid=? AND identity_key=?", (self.user_sid, identity.key))
        self.connection.commit()

    def decide_key(self, identity_key: str, state: str, project_ids: list[str] | tuple[str, ...]) -> None:
        if state not in {"allow_global", "allow_project", "deny", "always_ignore"}:
            raise ValueError("进程授权决定无效")
        if state == "allow_project" and not project_ids:
            raise ValueError("项目范围授权必须指定项目")
        serialized_projects = ",".join(sorted({str(item) for item in project_ids if str(item)}))
        cursor = self.connection.execute(
            "UPDATE process_consents SET state=?,project_ids=?,decided_at=strftime('%s','now'),last_seen=strftime('%s','now') WHERE user_sid=? AND identity_key=?",
            (state, serialized_projects, self.user_sid, identity_key),
        )
        if cursor.rowcount != 1:
            raise ValueError("进程身份不存在")
        self.connection.commit()

    def reset_key(self, identity_key: str) -> None:
        self.connection.execute("DELETE FROM process_consents WHERE user_sid=? AND identity_key=?", (self.user_sid, identity_key))
        self.connection.commit()

    def close(self) -> None:
        self.connection.close()

    def _excluded(self, identity: StableProcessIdentity) -> bool:
        value = f"{identity.name} {identity.executable}".casefold()
        return any(fragment in value for fragment in self.excluded_fragments)
