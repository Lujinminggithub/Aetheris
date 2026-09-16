import unittest
from datetime import datetime, timezone
from pathlib import Path


class CommandCorrelationTests(unittest.TestCase):
    def test_suppresses_only_exact_ai_shell_duplicate(self):
        from aetheris.command_correlation import CommandOriginCorrelator

        now = datetime(2026, 9, 14, 8, 0, 20, tzinfo=timezone.utc)
        records = [
            ({"event_type": "terminal.command", "payload": {"command_hash": "h1"}}, "core.terminal", Path("D:/repo")),
            ({"event_type": "ai.tool_call", "payload": {"tool": "codex", "tool_call_type": "shell", "command_hash": "h1", "project": "D:/repo", "timestamp": "2026-09-14T08:00:00Z"}}, "core.ai.codex", None),
            ({"event_type": "terminal.command", "payload": {"command_hash": "h2"}}, "core.terminal", Path("D:/repo")),
        ]

        result = CommandOriginCorrelator().filter(records, now=now)

        self.assertEqual([item[0]["event_type"] for item in result.records], ["ai.tool_call", "terminal.command"])
        self.assertEqual(result.records[-1][0]["payload"]["command_hash"], "h2")
        self.assertEqual(result.suppressed_count, 1)

    def test_parent_name_without_hash_does_not_suppress_direct_terminal_command(self):
        from aetheris.command_correlation import CommandOriginCorrelator

        result = CommandOriginCorrelator().filter([
            ({"event_type": "terminal.command", "payload": {"parent_process": "codex.exe"}}, "core.terminal", Path("D:/repo")),
        ], now=datetime(2026, 9, 14, 8, 0, tzinfo=timezone.utc))

        self.assertEqual(len(result.records), 1)
        self.assertEqual(result.suppressed_count, 0)

    def test_mismatch_project_or_window_is_retained(self):
        from aetheris.command_correlation import CommandOriginCorrelator

        result = CommandOriginCorrelator().filter([
            ({"event_type": "terminal.command", "payload": {"command_hash": "h1"}}, "core.terminal", Path("D:/other")),
            ({"event_type": "ai.tool_call", "payload": {"tool": "claude_code", "tool_call_type": "shell", "command_hash": "h1", "project": "D:/repo", "timestamp": "2026-09-14T07:59:00Z"}}, "core.ai.claude_code", None),
        ], now=datetime(2026, 9, 14, 8, 0, tzinfo=timezone.utc))

        self.assertEqual(len(result.records), 2)

    def test_same_capture_can_correlate_when_source_has_no_timestamp(self):
        from aetheris.command_correlation import CommandOriginCorrelator

        result = CommandOriginCorrelator().filter([
            ({"event_type": "terminal.command", "payload": {"command_hash": "h1"}}, "core.terminal", Path("D:/repo")),
            ({"event_type": "ai.tool_call", "payload": {"tool": "codex", "tool_call_type": "shell", "command_hash": "h1", "project": "D:/repo"}}, "core.ai.codex", None),
        ], now=datetime(2026, 9, 14, 8, 0, tzinfo=timezone.utc))

        self.assertEqual([item[0]["event_type"] for item in result.records], ["ai.tool_call"])
        self.assertEqual(result.suppressed_count, 1)

    def test_recent_ai_signal_suppresses_terminal_in_next_capture_cycle(self):
        from aetheris.command_correlation import CommandOriginCorrelator

        correlator = CommandOriginCorrelator()
        first = datetime(2026, 9, 14, 8, 0, tzinfo=timezone.utc)
        correlator.filter([
            ({"event_type": "ai.tool_call", "payload": {"tool": "claude_code", "tool_call_type": "shell", "command_hash": "h1", "project": "D:/repo"}}, "core.ai.claude_code", None),
        ], now=first)

        within = correlator.filter([
            ({"event_type": "terminal.command", "payload": {"command_hash": "h1"}}, "core.terminal", Path("D:/repo")),
        ], now=first.replace(second=15))
        expired = correlator.filter([
            ({"event_type": "terminal.command", "payload": {"command_hash": "h1"}}, "core.terminal", Path("D:/repo")),
        ], now=first.replace(second=45))

        self.assertEqual(within.records, [])
        self.assertEqual(within.suppressed_count, 1)
        self.assertEqual(len(expired.records), 1)

    def test_ai_cwd_subdirectory_matches_authorized_project_root(self):
        from aetheris.command_correlation import CommandOriginCorrelator

        result = CommandOriginCorrelator().filter([
            ({"event_type": "terminal.command", "payload": {"command_hash": "h1"}}, "core.terminal", Path("D:/repo")),
            ({"event_type": "ai.tool_call", "payload": {"tool": "codex", "tool_call_type": "shell", "command_hash": "h1", "project": "D:/repo/src/module"}}, "core.ai.codex", None),
        ], now=datetime(2026, 9, 14, 8, 0, tzinfo=timezone.utc), project_roots=[Path("D:/repo")])

        self.assertEqual([item[0]["event_type"] for item in result.records], ["ai.tool_call"])
