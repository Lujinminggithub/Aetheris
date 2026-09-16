from __future__ import annotations

import configparser
import hashlib
import hmac
import re
from dataclasses import dataclass
from pathlib import Path
from urllib.parse import unquote, urlparse


_SCP_REMOTE = re.compile(r"^(?:[^@/]+@)?(?P<host>[^:/]+):(?P<path>.+)$")


@dataclass(frozen=True)
class LocalProjectIdentity:
    local_project_id: str
    display_name: str
    vcs: str
    remote_fingerprint: str | None
    root_fingerprint: str
    workspace_kind: str
    worktree_name: str
    key_version: int

    def to_registration(self) -> dict[str, object]:
        return {
            "local_project_id": self.local_project_id,
            "display_name": self.display_name,
            "vcs": self.vcs,
            "remote_fingerprint": self.remote_fingerprint,
            "root_fingerprint": self.root_fingerprint,
            "workspace_kind": self.workspace_kind,
            "worktree_name": self.worktree_name,
            "active": True,
            "key_version": self.key_version,
        }


class ProjectIdentityHasher:
    def __init__(self, remote_key: bytes, root_key: bytes, *, key_version: int):
        if len(remote_key) < 32 or len(root_key) < 32:
            raise ValueError("项目身份密钥长度不得少于 32 字节")
        if key_version < 1:
            raise ValueError("项目身份密钥版本必须为正整数")
        self._remote_key = remote_key
        self._root_key = root_key
        self.key_version = key_version

    def remote_fingerprint(self, remote: str) -> str:
        canonical = normalize_git_remote(remote)
        if not canonical:
            raise ValueError("Git 远程地址无效")
        return self._fingerprint(self._remote_key, canonical)

    def root_fingerprint(self, root: str | Path) -> str:
        canonical = str(Path(root).expanduser().resolve()).casefold()
        return self._fingerprint(self._root_key, canonical)

    def _fingerprint(self, key: bytes, value: str) -> str:
        digest = hmac.new(key, value.encode("utf-8"), hashlib.sha256).hexdigest()
        return f"hmac-sha256:v{self.key_version}:{digest}"


def normalize_git_remote(value: str) -> str:
    remote = str(value).strip()
    if not remote:
        return ""
    match = _SCP_REMOTE.match(remote) if "://" not in remote else None
    if match:
        host = match.group("host").casefold().rstrip(".")
        path = match.group("path")
    else:
        parsed = urlparse(remote)
        if parsed.scheme.casefold() not in {"http", "https", "ssh", "git"} or not parsed.hostname:
            return ""
        host = parsed.hostname.casefold().rstrip(".")
        if parsed.port:
            host = f"{host}:{parsed.port}"
        path = parsed.path
    normalized_path = unquote(path).replace("\\", "/").strip("/")
    if normalized_path.casefold().endswith(".git"):
        normalized_path = normalized_path[:-4]
    normalized_path = "/".join(part for part in normalized_path.split("/") if part not in {"", "."})
    if not host or not normalized_path or ".." in normalized_path.split("/"):
        return ""
    return f"{host}/{normalized_path}"


def inspect_project_identity(
    path: str | Path,
    *,
    remote_key: bytes,
    root_key: bytes,
    key_version: int,
) -> LocalProjectIdentity:
    root = Path(path).expanduser().resolve()
    if not root.is_dir():
        raise ValueError("项目目录不存在或无法访问")
    git_marker = root / ".git"
    if git_marker.is_dir():
        vcs = "git"
        workspace_kind = "primary"
        worktree_name = ""
        config_path = git_marker / "config"
    elif git_marker.is_file():
        vcs = "git"
        workspace_kind = "worktree"
        worktree_name = root.name
        config_path = _worktree_config(git_marker)
    elif (root / ".svn").is_dir():
        vcs = "svn"
        workspace_kind = "primary"
        worktree_name = ""
        config_path = None
    else:
        vcs = "none"
        workspace_kind = "non_vcs"
        worktree_name = ""
        config_path = None

    hasher = ProjectIdentityHasher(remote_key, root_key, key_version=key_version)
    remote = _read_origin(config_path) if config_path else ""
    remote_fingerprint = hasher.remote_fingerprint(remote) if remote else None
    local_digest = hashlib.sha256(str(root).casefold().encode("utf-8")).hexdigest()[:16]
    return LocalProjectIdentity(
        local_project_id=f"project-{local_digest}",
        display_name=root.name,
        vcs=vcs,
        remote_fingerprint=remote_fingerprint,
        root_fingerprint=hasher.root_fingerprint(root),
        workspace_kind=workspace_kind,
        worktree_name=worktree_name,
        key_version=key_version,
    )


def _worktree_config(marker: Path) -> Path | None:
    try:
        line = marker.read_text(encoding="utf-8-sig", errors="replace").strip()
    except OSError:
        return None
    if not line.casefold().startswith("gitdir:"):
        return None
    git_dir = Path(line.split(":", 1)[1].strip())
    if not git_dir.is_absolute():
        git_dir = (marker.parent / git_dir).resolve()
    common_dir = git_dir
    common_marker = git_dir / "commondir"
    if common_marker.is_file():
        try:
            common_value = common_marker.read_text(encoding="utf-8-sig", errors="replace").strip()
            candidate = Path(common_value)
            common_dir = candidate if candidate.is_absolute() else (git_dir / candidate).resolve()
        except OSError:
            return None
    return common_dir / "config"


def _read_origin(config_path: Path) -> str:
    if not config_path.is_file():
        return ""
    parser = configparser.RawConfigParser(interpolation=None)
    try:
        parser.read(config_path, encoding="utf-8-sig")
        return parser.get('remote "origin"', "url", fallback="").strip()
    except (OSError, configparser.Error):
        return ""
