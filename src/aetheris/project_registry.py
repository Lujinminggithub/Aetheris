from __future__ import annotations

import json
import hashlib
import os
import threading
from dataclasses import dataclass, replace
from datetime import datetime, timezone
from pathlib import Path


class RevisionConflict(RuntimeError):
    pass


@dataclass(frozen=True)
class RegistryProject:
    path: Path
    vcs: str
    state: str = "active"
    classification: str = "identified"
    display_name: str = ""
    local_project_id: str = ""
    remote_fingerprint: str | None = None
    root_fingerprint: str | None = None
    workspace_kind: str = ""
    worktree_name: str = ""
    key_version: int = 0

    def to_dict(self) -> dict:
        value = {
            "path": str(self.path),
            "vcs": self.vcs,
            "state": self.state,
            "classification": self.classification,
            "display_name": self.display_name or self.path.name,
            "local_project_id": self.local_project_id or _local_project_id(self.path),
            "workspace_kind": self.workspace_kind or _workspace_kind(self.path, self.vcs),
            "worktree_name": self.worktree_name,
            "key_version": max(0, int(self.key_version)),
            "available": self.path.is_dir(),
        }
        if self.remote_fingerprint:
            value["remote_fingerprint"] = self.remote_fingerprint
        if self.root_fingerprint:
            value["root_fingerprint"] = self.root_fingerprint
        return value


@dataclass(frozen=True)
class ProjectSnapshot:
    revision: int
    projects: list[RegistryProject]


class ProjectRegistry:
    def __init__(self, config_path: str | Path):
        self.config_path = Path(config_path).expanduser().resolve()
        self.audit_path = self.config_path.parent.parent / "data" / "project-audit.jsonl"
        self._lock = threading.RLock()

    def load(self) -> ProjectSnapshot:
        with self._lock:
            data = self._read_config()
            projects = []
            for item in data.get("project_roots", []):
                if not isinstance(item, dict) or not item.get("path"):
                    continue
                path = Path(item["path"]).expanduser().resolve()
                vcs = str(item.get("vcs", "pending"))
                projects.append(RegistryProject(
                    path,
                    vcs,
                    str(item.get("state", "active")),
                    str(item.get("classification", "identified")),
                    str(item.get("display_name") or path.name),
                    str(item.get("local_project_id") or _local_project_id(path)),
                    str(item["remote_fingerprint"]) if item.get("remote_fingerprint") else None,
                    str(item["root_fingerprint"]) if item.get("root_fingerprint") else None,
                    str(item.get("workspace_kind") or _workspace_kind(path, vcs)),
                    str(item.get("worktree_name") or ""),
                    max(0, int(item.get("key_version", 0))),
                ))
            return ProjectSnapshot(int(data.get("project_revision", 0)), projects)

    def mutate(self, expected_revision: int, action: str, path: str | Path, vcs: str | None = None) -> ProjectSnapshot:
        with self._lock:
            data = self._read_config()
            revision = int(data.get("project_revision", 0))
            if revision != expected_revision:
                raise RevisionConflict(f"project revision changed: expected {expected_revision}, current {revision}")
            target = Path(path).expanduser().resolve()
            key = os.path.normcase(str(target))
            projects = list(self.load().projects)
            index = next((position for position, item in enumerate(projects) if os.path.normcase(str(item.path)) == key), None)
            if action == "add":
                if not target.is_dir():
                    raise ValueError("project path is not an accessible directory")
                detected = vcs or self._detect_vcs(target)
                classification = "identified" if detected in {"git", "svn"} else "pending_classification"
                project = RegistryProject(
                    target,
                    detected,
                    "active",
                    classification,
                    target.name,
                    _local_project_id(target),
                    workspace_kind=_workspace_kind(target, detected),
                    worktree_name=target.name if detected == "git" and (target / ".git").is_file() else "",
                )
                if index is None:
                    projects.append(project)
                else:
                    current = projects[index]
                    projects[index] = replace(
                        project,
                        remote_fingerprint=current.remote_fingerprint,
                        root_fingerprint=current.root_fingerprint,
                        key_version=current.key_version,
                    )
            elif action in {"pause", "resume"}:
                if index is None:
                    raise ValueError("project is not registered")
                current = projects[index]
                projects[index] = replace(current, state="paused" if action == "pause" else "active")
            elif action == "remove":
                if index is None:
                    raise ValueError("project is not registered")
                projects.pop(index)
            else:
                raise ValueError("unsupported project operation")
            projects.sort(key=lambda item: os.path.normcase(str(item.path)))
            data["project_revision"] = revision + 1
            data["project_roots"] = [item.to_dict() for item in projects]
            data["authorized_roots"] = [str(item.path) for item in projects]
            active = [item for item in projects if item.state == "active" and item.vcs in {"git", "svn"} and item.path.is_dir()]
            if active:
                data["project_root"] = str(active[0].path)
            else:
                data.pop("project_root", None)
            self._atomic_json(data)
            self._append_audit(action, target, revision + 1)
            return ProjectSnapshot(revision + 1, projects)

    def _read_config(self) -> dict:
        try:
            value = json.loads(self.config_path.read_text(encoding="utf-8-sig"))
        except (OSError, json.JSONDecodeError) as exc:
            raise ValueError("project configuration is unreadable") from exc
        if not isinstance(value, dict):
            raise ValueError("project configuration must be an object")
        return value

    def _atomic_json(self, data: dict) -> None:
        temporary = self.config_path.with_suffix(self.config_path.suffix + ".tmp")
        temporary.write_text(json.dumps(data, ensure_ascii=True, indent=2), encoding="utf-8", newline="\n")
        temporary.replace(self.config_path)

    def _append_audit(self, action: str, path: Path, revision: int) -> None:
        self.audit_path.parent.mkdir(parents=True, exist_ok=True)
        record = {"action": action, "path": str(path), "revision": revision, "timestamp": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")}
        with self.audit_path.open("a", encoding="utf-8", newline="\n") as handle:
            handle.write(json.dumps(record, ensure_ascii=True, separators=(",", ":")) + "\n")

    @staticmethod
    def _detect_vcs(path: Path) -> str:
        if (path / ".git").exists():
            return "git"
        if (path / ".svn").is_dir():
            return "svn"
        return "pending"


def _local_project_id(path: Path) -> str:
    digest = hashlib.sha256(str(path.expanduser().resolve()).casefold().encode("utf-8")).hexdigest()[:16]
    return f"project-{digest}"


def _workspace_kind(path: Path, vcs: str) -> str:
    if vcs == "git" and (path / ".git").is_file():
        return "worktree"
    if vcs in {"git", "svn"}:
        return "primary"
    return "non_vcs"
