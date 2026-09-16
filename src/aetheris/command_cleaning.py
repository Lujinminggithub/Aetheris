from __future__ import annotations

import base64
import os
import subprocess
from dataclasses import dataclass
from typing import Callable


@dataclass(frozen=True)
class LogicalCommand:
    command: str
    line_start: int
    line_end: int
    merge_method: str
    quality_state: str = "accepted"
    reason_codes: list[str] | None = None
    excluded_from_effectiveness: bool = False

    def __post_init__(self) -> None:
        if self.reason_codes is None:
            object.__setattr__(self, "reason_codes", [])


def powershell_syntax_complete(command: str) -> bool:
    if os.name != "nt" or not command.strip():
        return False
    script = (
        "$source=[Console]::In.ReadToEnd();$tokens=$null;$errors=$null;"
        "[System.Management.Automation.Language.Parser]::ParseInput($source,[ref]$tokens,[ref]$errors)|Out-Null;"
        "if($errors.Count -eq 0){exit 0}else{exit 1}"
    )
    encoded = base64.b64encode(script.encode("utf-16le")).decode("ascii")
    try:
        result = subprocess.run(
            ["powershell.exe", "-NoProfile", "-NonInteractive", "-EncodedCommand", encoded],
            input=command,
            text=True,
            capture_output=True,
            timeout=5,
            creationflags=subprocess.CREATE_NO_WINDOW,
        )
        return result.returncode == 0
    except (OSError, subprocess.SubprocessError):
        return False


def assemble_logical_commands(
    physical_lines: list[str],
    *,
    syntax_checker: Callable[[str], bool] = powershell_syntax_complete,
) -> list[LogicalCommand]:
    grouped: list[LogicalCommand] = []
    index = 0
    while index < len(physical_lines):
        start = index + 1
        parts: list[str] = []
        explicit = False
        while index < len(physical_lines):
            line = physical_lines[index].rstrip("\r\n")
            stripped = line.rstrip()
            continued = _has_line_continuation(stripped)
            if continued:
                explicit = True
                stripped = stripped[:-1].rstrip()
            parts.append(stripped.strip())
            index += 1
            if not continued:
                break
        command = " ".join(part for part in parts if part).strip()
        if not command:
            continue
        if explicit and not syntax_checker(command):
            grouped.append(LogicalCommand(command, start, index, "explicit_backtick_join", "quarantined", ["powershell_parse_error"], True))
        else:
            grouped.append(LogicalCommand(command, start, index, "explicit_backtick_join" if explicit else "none", reason_codes=["explicit_backtick_join"] if explicit else []))

    result: list[LogicalCommand] = []
    index = 0
    while index < len(grouped):
        current = grouped[index]
        if current.quality_state == "quarantined":
            result.append(current)
            index += 1
            continue
        if _is_parameter_fragment(current.command):
            if index + 1 < len(grouped) and not _is_parameter_fragment(grouped[index + 1].command):
                primary = grouped[index + 1]
                candidate = f"{primary.command} {current.command}".strip()
                if syntax_checker(candidate):
                    result.append(LogicalCommand(candidate, current.line_start, primary.line_end, "inferred_parameter_join", reason_codes=["orphan_parameter_fragment", "inferred_reordered_join"]))
                    index += 2
                    continue
            result.append(LogicalCommand(current.command, current.line_start, current.line_end, "none", "quarantined", ["orphan_parameter_fragment"], True))
            index += 1
            continue
        if index + 1 < len(grouped) and _is_parameter_fragment(grouped[index + 1].command):
            fragment = grouped[index + 1]
            if index + 2 < len(grouped) and not _is_parameter_fragment(grouped[index + 2].command):
                result.append(current)
                index += 1
                continue
            candidate = f"{current.command} {fragment.command}".strip()
            if syntax_checker(candidate):
                result.append(LogicalCommand(candidate, current.line_start, fragment.line_end, "inferred_parameter_join", reason_codes=["orphan_parameter_fragment", "inferred_parameter_join"]))
                index += 2
                continue
        result.append(current)
        index += 1
    return result


def _is_parameter_fragment(command: str) -> bool:
    return command.lstrip().startswith("-")


def _has_line_continuation(command: str) -> bool:
    trailing = len(command) - len(command.rstrip("`"))
    return trailing % 2 == 1
