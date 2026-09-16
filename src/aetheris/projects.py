from __future__ import annotations

import os
import stat
from dataclasses import dataclass
from pathlib import Path
from typing import Literal


SKIP_DIRECTORIES = {
    ".git", ".svn", "node_modules", ".venv", "venv", "dist", "build",
    ".cache", "__pycache__", "bin", "obj",
}


@dataclass(frozen=True)
class ProjectSource:
    path: Path
    vcs: Literal["git", "svn"]

    def to_dict(self) -> dict[str, str]:
        return {"path": str(self.path), "vcs": self.vcs}


@dataclass(frozen=True)
class ScanResult:
    projects: list[ProjectSource]
    truncated: bool = False


def discover_projects(root: str | Path, *, max_depth: int = 8, limit: int = 100) -> ScanResult:
    start = Path(root).expanduser().resolve()
    if not start.is_dir():
        raise ValueError(f"扫描根目录不存在: {start}")
    if max_depth < 0 or limit < 1:
        raise ValueError("扫描深度和结果上限必须为正数")
    found: list[ProjectSource] = []
    seen: set[str] = set()
    truncated = False

    def visit(current: Path, depth: int) -> None:
        nonlocal truncated
        if depth > max_depth or truncated:
            return
        vcs = _vcs_marker(current)
        if vcs:
            key = os.path.normcase(str(current.resolve()))
            if key not in seen:
                if len(found) >= limit:
                    truncated = True
                    return
                seen.add(key)
                found.append(ProjectSource(current.resolve(), vcs))
        try:
            entries = sorted(os.scandir(current), key=lambda item: item.name.casefold())
        except (OSError, PermissionError):
            return
        for entry in entries:
            if entry.name in SKIP_DIRECTORIES or entry.name.casefold() in SKIP_DIRECTORIES:
                continue
            try:
                if not entry.is_dir(follow_symlinks=False) or entry.is_symlink() or _is_reparse_point(entry):
                    continue
            except OSError:
                continue
            visit(Path(entry.path), depth + 1)
            if truncated:
                return

    visit(start, 0)
    found.sort(key=lambda item: os.path.normcase(str(item.path)))
    return ScanResult(found, truncated)


def _vcs_marker(path: Path) -> Literal["git", "svn"] | None:
    git = path / ".git"
    if git.is_dir() or git.is_file():
        return "git"
    if (path / ".svn").is_dir():
        return "svn"
    return None


def _is_reparse_point(entry: os.DirEntry[str]) -> bool:
    try:
        attributes = getattr(entry.stat(follow_symlinks=False), "st_file_attributes", 0)
    except OSError:
        return True
    return bool(attributes & getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 0x400))

