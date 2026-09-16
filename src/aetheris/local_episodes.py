from __future__ import annotations

from collections import defaultdict
from datetime import datetime, timezone


_IDE_ACTIONS = {
    "ide.file_opened": "打开",
    "ide.file_edited": "编辑",
    "ide.file_saved": "保存",
    "ide.file_closed": "关闭",
}
_LANGUAGE_LABELS = {
    "typescript": "TypeScript", "typescriptreact": "TypeScript",
    "javascript": "JavaScript", "javascriptreact": "JavaScript",
    "python": "Python", "go": "Go", "java": "Java", "csharp": "C#", "cpp": "C++", "rust": "Rust",
}


def build_local_episodes(events: list[dict], *, device_id: str, max_events: int = 500, project_names: dict[str, str] | None = None) -> list[dict]:
    project_names = project_names or {}
    selected = [event for event in events if event.get("device_id") == device_id][:max_events]
    groups: dict[tuple[str, str], list[dict]] = defaultdict(list)
    for event in selected:
        groups[(str(event.get("project_id", "")), str(event.get("session_id", "")))].append(event)
    result = []
    for (project_id, session_id), group in groups.items():
        group.sort(key=lambda event: event.get("occurred_at", ""))
        objective = ""
        actions = []
        event_ids = []
        for event in group:
            event_ids.append(str(event.get("event_id", "")))
            payload = event.get("payload") if isinstance(event.get("payload"), dict) else {}
            if event.get("event_type") == "ai.message" and payload.get("role") == "user" and not objective:
                candidate = str(payload.get("content", ""))
                if not candidate.lstrip().startswith("<environment_context>"):
                    objective = candidate[:240]
            event_type = str(event.get("event_type", ""))
            if event_type in {"ai.tool_call", "terminal.command", "ide.activity", "git.commit", "git.diff"}:
                summary = payload.get("command_summary") or payload.get("summary")
                if summary:
                    actions.append({"event_id": event.get("event_id", ""), "summary": str(summary)[:240]})
            elif event_type in _IDE_ACTIONS:
                language = _LANGUAGE_LABELS.get(str(payload.get("language_id", "")).casefold(), "代码")
                actions.append({"event_id": event.get("event_id", ""), "summary": f"{_IDE_ACTIONS[event_type]} {language} 文件"})
            elif event_type == "ide.workspace_changed":
                actions.append({"event_id": event.get("event_id", ""), "summary": "切换工作区"})
            elif event_type == "ide.extension_changed":
                actions.append({"event_id": event.get("event_id", ""), "summary": "开发扩展环境变化"})
        if not objective and not actions:
            continue
        result.append({
            "episode_id": f"local-episode-{project_id}-{session_id}",
            "project_id": project_id,
            "project_name": project_names.get(project_id, project_id or "未归属项目"),
            "device_id": device_id,
            "session_id": session_id,
            "objective": objective,
            "actions": actions,
            "event_ids": event_ids,
            "started_at": group[0].get("occurred_at"),
            "ended_at": group[-1].get("occurred_at"),
            "needs_review": not bool(objective),
            "event_count": len(event_ids),
        })
    return sorted(result, key=lambda item: item.get("started_at") or "", reverse=True)
