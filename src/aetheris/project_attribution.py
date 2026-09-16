from __future__ import annotations

import hashlib
import re
from dataclasses import dataclass
from pathlib import Path

from .core import ProjectResolver


@dataclass(frozen=True)
class AIProjectAttribution:
    project_id: str
    project_label: str
    attribution: str
    project_root: Path | None


_TOOL_LABELS = {
    "codex": "Codex",
    "claude_code": "Claude Code",
    "cursor": "Cursor",
    "github_copilot": "GitHub Copilot",
}


def attribute_ai_project(
    tool: str,
    cwd: str | Path | None,
    authorized_roots: list[str | Path],
    *,
    namespace: str,
) -> AIProjectAttribution:
    normalized_tool = _normalize_tool(tool)
    if cwd:
        try:
            project = ProjectResolver().resolve(cwd, authorized_roots)
        except (OSError, ValueError):
            project = None
        if project is not None:
            return AIProjectAttribution(project.project_id, project.root.name or normalized_tool, "authorized_root", project.root)

    label = _TOOL_LABELS.get(normalized_tool, _humanize_tool(normalized_tool))
    digest = hashlib.sha256(f"{namespace}\0ai-tool\0{normalized_tool}".encode("utf-8")).hexdigest()[:16]
    return AIProjectAttribution(f"project-{digest}", label, "tool_fallback", None)


def _normalize_tool(tool: str) -> str:
    value = re.sub(r"[^a-z0-9]+", "_", str(tool).strip().casefold()).strip("_")
    return value or "ai"


def _humanize_tool(tool: str) -> str:
    return " ".join(part.capitalize() for part in tool.split("_") if part) or "AI"
