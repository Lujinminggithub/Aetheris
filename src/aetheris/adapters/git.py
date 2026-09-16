from __future__ import annotations

import hashlib
import os
import subprocess
from pathlib import Path


class GitAdapter:
    """Collect safe Git metadata without reading patch bodies."""

    def collect(self, repo: str | Path) -> list[dict]:
        root = Path(repo).expanduser().resolve()
        commit = self._run(
            root,
            ["git", "log", "-1", "--format=%H%x00%an%x00%ae%x00%aI%x00%s"],
        )
        if not commit:
            return []
        parts = commit.rstrip("\r\n").split("\x00")
        if len(parts) != 5:
            raise RuntimeError("unexpected git log format")
        commit_hash, author_name, author_email, authored_at, subject = parts
        email_hash = hashlib.sha256(author_email.casefold().encode("utf-8")).hexdigest()[:16]
        numstat = self._run(root, ["git", "diff", "--numstat"])
        files_changed = 0
        insertions = 0
        deletions = 0
        for line in numstat.splitlines():
            columns = line.split("\t")
            if len(columns) >= 3:
                files_changed += 1
                insertions += int(columns[0]) if columns[0].isdigit() else 0
                deletions += int(columns[1]) if columns[1].isdigit() else 0
        return [
            {
                "event_type": "git.commit",
                "payload": {
                    "commit_hash": commit_hash,
                    "author_name": author_name,
                    "author_email_hash": email_hash,
                    "authored_at": authored_at,
                    "subject": subject,
                },
            },
            {
                "event_type": "git.diff",
                "payload": {
                    "files_changed": files_changed,
                    "insertions": insertions,
                    "deletions": deletions,
                },
            },
        ]

    @staticmethod
    def _run(root: Path, command: list[str]) -> str:
        kwargs = {"cwd": root, "check": True, "capture_output": True, "text": True, "timeout": 10}
        if os.name == "nt":
            kwargs["creationflags"] = subprocess.CREATE_NO_WINDOW
        result = subprocess.run(command, **kwargs)
        return result.stdout
