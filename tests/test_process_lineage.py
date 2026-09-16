import unittest
import os
from unittest.mock import patch


class ProcessLineageTests(unittest.TestCase):
    def test_ai_ancestor_marks_shell_as_ai_origin(self):
        from aetheris.process_lineage import classify_shell_origin

        self.assertEqual(classify_shell_origin("powershell.exe", ["powershell.exe", "node.exe", "codex.exe"]), "ai")

    def test_unknown_or_unrelated_parent_does_not_mark_ai_origin(self):
        from aetheris.process_lineage import classify_shell_origin

        self.assertEqual(classify_shell_origin("powershell.exe", ["powershell.exe", "explorer.exe"]), "unknown")
        self.assertEqual(classify_shell_origin("powershell.exe", ["powershell.exe", "claude-helper.exe"]), "unknown")

    def test_lineage_projection_contains_only_process_identity(self):
        from aetheris.process_lineage import ProcessLineage

        value = ProcessLineage(22, 11, ("powershell.exe", "codex.exe"), "ai").to_payload()

        self.assertEqual(value, {"parent_pid": 11, "ancestor_processes": ["powershell.exe", "codex.exe"], "origin": "ai"})
        self.assertNotIn("command", str(value))

    @unittest.skipUnless(os.name == "nt", "仅 Windows 支持原生进程父链")
    def test_native_provider_reads_current_process_without_shell(self):
        from aetheris.process_lineage import NativeProcessLineageProvider

        value = NativeProcessLineageProvider().inspect(os.getpid(), "python.exe")

        self.assertEqual(value.pid, os.getpid())
        self.assertGreater(value.parent_pid, 0)
        self.assertEqual(value.ancestor_processes[0], "python.exe")

    def test_lineage_walk_uses_parent_name_without_repeating_child(self):
        from aetheris.process_lineage import build_lineage

        value = build_lineage(30, "powershell.exe", {
            30: (20, "powershell.exe"), 20: (10, "node.exe"), 10: (1, "codex.exe"), 1: (0, "explorer.exe"),
        })

        self.assertEqual(value.ancestor_processes, ("powershell.exe", "node.exe", "codex.exe", "explorer.exe"))
        self.assertEqual(value.origin, "ai")

    @unittest.skipUnless(os.name == "nt", "仅 Windows 支持原生进程父链")
    def test_native_snapshot_failure_returns_unknown_lineage(self):
        from aetheris.process_lineage import NativeProcessLineageProvider

        with patch("aetheris.process_lineage._process_snapshot", side_effect=OSError("denied")):
            value = NativeProcessLineageProvider().inspect(22, "cmd.exe")

        self.assertEqual(value.parent_pid, 0)
        self.assertEqual(value.origin, "unknown")
