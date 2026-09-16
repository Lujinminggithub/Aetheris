from __future__ import annotations

import hashlib
import os
import subprocess
import xml.etree.ElementTree as ET
from collections import Counter
from pathlib import Path


class SvnAdapter:
    """采集 SVN 工作副本元数据，不返回仓库 URL、凭据或文件内容。"""

    def collect(self, root: str | Path) -> list[dict]:
        project = Path(root).expanduser().resolve()
        try:
            info_root = ET.fromstring(self._run(project, ["svn", "info", "--xml"]))
            status_root = ET.fromstring(self._run(project, ["svn", "status", "--xml"]))
        except (FileNotFoundError, subprocess.SubprocessError, ET.ParseError):
            return []
        entry = info_root.find("entry")
        if entry is None:
            return []
        repository_uuid = entry.findtext("repository/uuid", default="")
        commit = entry.find("commit")
        status_counts = Counter(
            node.attrib.get("item", "unknown")
            for node in status_root.findall(".//wc-status")
        )
        return [{
            "event_type": "svn.activity",
            "payload": {
                "working_copy_revision": entry.attrib.get("revision", ""),
                "last_changed_revision": commit.attrib.get("revision", "") if commit is not None else "",
                "last_changed_author": commit.findtext("author", default="") if commit is not None else "",
                "last_changed_at": commit.findtext("date", default="") if commit is not None else "",
                "repository_uuid_hash": hashlib.sha256(repository_uuid.encode("utf-8")).hexdigest()[:16] if repository_uuid else "",
                "status_counts": dict(sorted(status_counts.items())),
            },
        }]

    @staticmethod
    def _run(root: Path, command: list[str]) -> str:
        kwargs = {"cwd": root, "check": True, "capture_output": True, "text": True, "timeout": 10}
        if os.name == "nt":
            kwargs["creationflags"] = subprocess.CREATE_NO_WINDOW
        return subprocess.run(command, **kwargs).stdout

