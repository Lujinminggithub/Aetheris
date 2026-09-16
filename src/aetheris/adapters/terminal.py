from __future__ import annotations

import os
from pathlib import Path

from ..redaction import Redactor
from ..command_cleaning import assemble_logical_commands, powershell_syntax_complete
from ..command_privacy import CommandPrivacy


class TerminalAdapter:
    """Read only newly appended PowerShell/Windows Terminal history lines."""

    def __init__(self, history_path: str | Path | None = None, max_command_length: int = 4000, syntax_checker=powershell_syntax_complete, command_privacy: CommandPrivacy | None = None):
        self.history_path = Path(history_path) if history_path else self.default_history_path()
        self.max_command_length = max_command_length
        self._offset = 0
        self._redactor = Redactor()
        self._syntax_checker = syntax_checker
        self._line_number = 0
        self._command_privacy = command_privacy

    @staticmethod
    def default_history_path() -> Path:
        appdata = os.environ.get("APPDATA")
        if appdata:
            return Path(appdata) / "Microsoft" / "Windows" / "PowerShell" / "PSReadLine" / "ConsoleHost_history.txt"
        return Path.home() / ".aetheris" / "ConsoleHost_history.txt"

    def collect(self) -> list[dict]:
        if not self.history_path.is_file():
            return []
        try:
            with self.history_path.open("r", encoding="utf-8-sig", errors="replace") as handle:
                handle.seek(self._offset)
                lines = handle.readlines()
                self._offset = handle.tell()
        except OSError:
            return []
        records = []
        base_line = self._line_number
        self._line_number += len(lines)
        for logical in assemble_logical_commands(lines, syntax_checker=self._syntax_checker):
            command = logical.command.strip()
            if not command or command.startswith("#"):
                continue
            command = command[: self.max_command_length]
            safe_command, report = self._redactor.redact(command)
            descriptor = self._command_privacy.describe(command) if self._command_privacy else None
            descriptor_fields = {
                "command_type": descriptor.command_type,
                "command_summary": descriptor.command_summary,
                "command_hash": descriptor.command_hash,
            } if descriptor else {}
            records.append(
                {
                    "event_type": "terminal.command",
                    "payload": {
                        "command": safe_command,
                        "actor_origin": "unknown",
                        "merge_method": logical.merge_method,
                        "quality_state": logical.quality_state,
                        "reason_codes": logical.reason_codes,
                        "excluded_from_effectiveness": logical.excluded_from_effectiveness,
                        **descriptor_fields,
                    },
                    "redaction_report": report,
                    "provenance": {
                        "source_file": self.history_path.name,
                        "source_line_start": base_line + logical.line_start,
                        "source_line_end": base_line + logical.line_end,
                    },
                }
            )
        return records
