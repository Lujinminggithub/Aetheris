from __future__ import annotations

import json
import re
from pathlib import Path

from ..redaction import Redactor
from ..command_privacy import CommandPrivacy


class AISessionAdapter:
    """Parse only structured local JSONL sessions for an explicitly selected tool."""

    def __init__(
        self,
        root: str | Path,
        tool: str,
        max_files: int = 500,
        max_records: int = 500,
        checkpoint_path: str | Path | None = None,
        command_privacy: CommandPrivacy | None = None,
        live_records: int = 100,
        max_records_per_file: int = 20,
        live_files: int = 5,
    ):
        self.root = Path(root).expanduser().resolve()
        self.tool = tool
        self.max_files = max_files
        self.max_records = max_records
        self.live_records = max(0, min(int(live_records), self.max_records))
        self.max_records_per_file = max(1, int(max_records_per_file))
        self.live_files = max(0, int(live_files))
        self.checkpoint_path = Path(checkpoint_path).expanduser().resolve() if checkpoint_path else None
        self._offsets: dict[Path, int] = {}
        self._session_meta: dict[Path, dict] = {}
        self._backfill_cursor: Path | None = None
        self._progress = {"total_files": 0, "covered_files": 0, "pending_files": 0}
        self._redactor = Redactor()
        self._command_privacy = command_privacy
        self._load_checkpoint()

    def _load_checkpoint(self) -> None:
        if not self.checkpoint_path or not self.checkpoint_path.is_file():
            return
        try:
            data = json.loads(self.checkpoint_path.read_text(encoding="utf-8"))
            if data.get("tool") != self.tool:
                return
            self._offsets = {
                Path(raw_path).resolve(): int(offset)
                for raw_path, offset in data.get("offsets", {}).items()
                if isinstance(raw_path, str) and isinstance(offset, int) and offset >= 0
            }
            self._session_meta = {
                Path(raw_path).resolve(): metadata
                for raw_path, metadata in data.get("session_meta", {}).items()
                if isinstance(raw_path, str) and isinstance(metadata, dict)
            }
            raw_cursor = data.get("backfill_cursor")
            self._backfill_cursor = Path(raw_cursor).resolve() if isinstance(raw_cursor, str) and raw_cursor else None
        except (OSError, ValueError, TypeError, json.JSONDecodeError):
            self._offsets = {}
            self._session_meta = {}

    def save_checkpoint(self) -> None:
        if not self.checkpoint_path:
            return
        data = {
            "version": 2,
            "tool": self.tool,
            "offsets": {str(path): offset for path, offset in self._offsets.items()},
            "session_meta": {str(path): metadata for path, metadata in self._session_meta.items()},
            "backfill_cursor": str(self._backfill_cursor) if self._backfill_cursor else "",
        }
        self.checkpoint_path.parent.mkdir(parents=True, exist_ok=True)
        temporary = self.checkpoint_path.with_suffix(self.checkpoint_path.suffix + ".tmp")
        temporary.write_text(json.dumps(data, ensure_ascii=True, indent=2), encoding="utf-8", newline="\n")
        temporary.replace(self.checkpoint_path)

    def reset_history(self) -> None:
        """Rewind structured history after the server data generation decreased."""
        self._offsets = {}
        self._session_meta = {}
        self._backfill_cursor = None
        self._progress = {"total_files": 0, "covered_files": 0, "pending_files": 0}
        self.save_checkpoint()

    @property
    def progress(self) -> dict[str, int]:
        return dict(self._progress)

    def collect(self) -> list[dict]:
        if not self.root.is_dir():
            return []
        records: list[dict] = []
        try:
            paths = sorted(
                self.root.rglob("*.jsonl"),
                key=lambda candidate: candidate.stat().st_mtime_ns,
                reverse=True,
            )[: self.max_files]
        except OSError:
            return []
        live_paths = paths[: self.live_files]
        self._collect_paths(live_paths, self.live_records, records)

        remaining = self.max_records - len(records)
        live_set = set(live_paths)
        backfill_paths = sorted((path for path in paths if path not in live_set), key=lambda path: str(path).casefold())
        if remaining > 0 and backfill_paths:
            start = 0
            if self._backfill_cursor in backfill_paths:
                start = (backfill_paths.index(self._backfill_cursor) + 1) % len(backfill_paths)
            rotated = backfill_paths[start:] + backfill_paths[:start]
            for path in rotated:
                if len(records) >= self.max_records:
                    break
                self._collect_path(path, min(self.max_records_per_file, self.max_records - len(records)), records)
                self._backfill_cursor = path

        covered = 0
        for path in paths:
            try:
                covered += self._offsets.get(path, 0) >= path.stat().st_size
            except OSError:
                continue
        self._progress = {"total_files": len(paths), "covered_files": covered, "pending_files": max(0, len(paths) - covered)}
        return records

    def _collect_paths(self, paths: list[Path], budget: int, records: list[dict]) -> None:
        target = min(self.max_records, len(records) + max(0, budget))
        for path in paths:
            if len(records) >= target:
                break
            self._collect_path(path, min(self.max_records_per_file, target - len(records)), records)

    def _collect_path(self, path: Path, record_limit: int, records: list[dict]) -> None:
        if record_limit <= 0:
            return
        emitted = 0
        offset = self._offsets.get(path, 0)
        try:
            with path.open("r", encoding="utf-8-sig", errors="replace") as handle:
                if offset > path.stat().st_size:
                    offset = 0
                    self._session_meta.pop(path, None)
                handle.seek(offset)
                while emitted < record_limit:
                    source_offset = handle.tell()
                    line = handle.readline()
                    if not line:
                        break
                    self._offsets[path] = handle.tell()
                    try:
                        item = json.loads(line)
                    except json.JSONDecodeError:
                        continue
                    if self.tool == "codex" and item.get("type") == "session_meta" and isinstance(item.get("payload"), dict):
                        payload = item["payload"]
                        self._session_meta[path] = {
                            "session_id": payload.get("session_id") or payload.get("id"),
                            "project": payload.get("cwd"),
                        }
                        continue
                    for tool_record in self._parse_tool_calls(path, item, source_offset):
                        records.append(tool_record)
                        emitted += 1
                        if emitted >= record_limit:
                            break
                    if emitted >= record_limit:
                        break
                    record = self._parse_item(path, item, source_offset)
                    if record is not None:
                        records.append(record)
                        emitted += 1
        except OSError:
            return

    def _parse_tool_calls(self, path: Path, item: dict, source_offset: int) -> list[dict]:
        if self._command_privacy is None:
            return []
        candidates: list[tuple[str, str, str, dict, str]] = []
        payload = item.get("payload")
        if self.tool == "codex" and isinstance(payload, dict) and payload.get("type") in {"function_call", "custom_tool_call"}:
            name = str(payload.get("name") or "")
            if name.casefold() in {"shell_command", "exec_command", "powershell", "bash"}:
                arguments = payload.get("arguments") or payload.get("input")
                if isinstance(arguments, str):
                    try:
                        arguments = json.loads(arguments)
                    except json.JSONDecodeError:
                        arguments = {"command": arguments}
                if isinstance(arguments, dict):
                    command = arguments.get("command") or arguments.get("cmd")
                    if isinstance(command, list):
                        command = " ".join(str(part) for part in command)
                    if isinstance(command, str) and command.strip():
                        candidates.append((name, str(payload.get("call_id") or payload.get("id") or ""), command, self._session_meta.get(path, {}), "shell"))
            elif name.casefold() == "exec" and isinstance(payload.get("input"), str) and payload["input"].strip():
                message_id = str(payload.get("call_id") or payload.get("id") or "")
                commands = _extract_static_exec_commands(payload["input"])
                if commands:
                    for command_index, command in enumerate(commands):
                        command_id = message_id if len(commands) == 1 else f"{message_id}:{command_index}"
                        candidates.append((name, command_id, command, self._session_meta.get(path, {}), "shell"))
                else:
                    candidates.append((name, message_id, payload["input"], self._session_meta.get(path, {}), "automation"))
        message = item.get("message")
        if self.tool == "claude_code" and isinstance(message, dict) and isinstance(message.get("content"), list):
            for part in message["content"]:
                if not isinstance(part, dict) or part.get("type") != "tool_use" or str(part.get("name", "")).casefold() not in {"bash", "powershell", "shell"}:
                    continue
                tool_input = part.get("input")
                command = tool_input.get("command") if isinstance(tool_input, dict) else None
                if isinstance(command, str) and command.strip():
                    candidates.append((str(part.get("name")), str(part.get("id") or ""), command, {"session_id": item.get("sessionId"), "project": item.get("cwd") or item.get("project")}, "shell"))
        records = []
        for name, message_id, command, metadata, call_type in candidates:
            descriptor = self._command_privacy.describe(command)
            records.append({
                "event_type": "ai.tool_call",
                "payload": {
                    "tool": self.tool,
                    "tool_call_type": call_type,
                    "command_type": descriptor.command_type,
                    "command_summary": descriptor.command_summary,
                    "command_hash": descriptor.command_hash,
                    "session_id": metadata.get("session_id"),
                    "message_id": message_id,
                    "actor_origin": "ai",
                    "project": metadata.get("project"),
                    "timestamp": item.get("timestamp"),
                },
                "redaction_report": {"rules": [], "replacement_count": 0},
                "provenance": {"source_file": path.relative_to(self.root).as_posix(), "source_offset": source_offset, "tool_name": name},
            })
        return records

    def _parse_item(self, path: Path, item: dict, source_offset: int) -> dict | None:
        role = item.get("role")
        content = item.get("content") or item.get("text")
        metadata = {}
        if self.tool == "claude_code" and item.get("display"):
            role = "user"
            content = item.get("display")
            metadata = {"session_id": item.get("sessionId"), "project": item.get("project"), "timestamp": item.get("timestamp")}
        elif self.tool == "codex" and item.get("type") == "response_item" and isinstance(item.get("payload"), dict):
            payload = item["payload"]
            role = payload.get("role")
            raw_content = payload.get("content")
            if isinstance(raw_content, list):
                content = "\n".join(str(part.get("text", "")) for part in raw_content if isinstance(part, dict))
            else:
                content = raw_content
            metadata = {**self._session_meta.get(path, {}), "timestamp": item.get("timestamp")}
        elif self.tool == "claude_code" and isinstance(item.get("message"), dict):
            message = item["message"]
            role = message.get("role") or item.get("type")
            raw_content = message.get("content")
            if isinstance(raw_content, list):
                content = "\n".join(str(part.get("text", "")) for part in raw_content if isinstance(part, dict))
            else:
                content = raw_content
            metadata = {"session_id": item.get("sessionId"), "project": item.get("cwd") or item.get("project")}
        if role not in {"user", "assistant", "system", "tool"} or not isinstance(content, str) or not content:
            return None
        safe, report = self._redactor.redact(content)
        try:
            source_file = path.relative_to(self.root).as_posix()
        except ValueError:
            source_file = path.name
        return {
            "event_type": "ai.message",
            "payload": {"tool": self.tool, "role": role, "content": safe, **metadata},
            "redaction_report": report,
            "provenance": {"source_file": source_file, "source_offset": source_offset},
        }


_EXEC_CALL = re.compile(r"\btools\.(?:exec_command|shell_command)\s*\(")
_STATIC_CMD = re.compile(r'''(?:["']?cmd["']?)\s*:\s*("(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|`(?:\\.|[^`\\])*`)''', re.DOTALL)


def _extract_static_exec_commands(source: str) -> list[str]:
    commands: list[str] = []
    for match in _EXEC_CALL.finditer(source):
        call = _balanced_call(source, match.end() - 1)
        if call is None:
            continue
        command_match = _STATIC_CMD.search(call)
        if not command_match:
            continue
        command = _decode_static_js_string(command_match.group(1))
        if command and command.strip():
            commands.append(command)
    return commands


def _balanced_call(source: str, open_index: int) -> str | None:
    depth = 0
    quote = ""
    escaped = False
    for index in range(open_index, len(source)):
        char = source[index]
        if quote:
            if escaped:
                escaped = False
            elif char == "\\":
                escaped = True
            elif char == quote:
                quote = ""
            continue
        if char in {'"', "'", "`"}:
            quote = char
        elif char == "(":
            depth += 1
        elif char == ")":
            depth -= 1
            if depth == 0:
                return source[open_index + 1:index]
    return None


def _decode_static_js_string(literal: str) -> str | None:
    if literal.startswith('"'):
        try:
            value = json.loads(literal)
            return value if isinstance(value, str) else None
        except json.JSONDecodeError:
            return None
    body = literal[1:-1]
    if literal.startswith("`") and "${" in body:
        return None
    quote = literal[0]
    return body.replace(f"\\{quote}", quote).replace("\\\\", "\\")
