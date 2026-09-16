import tempfile
import unittest
from pathlib import Path


class TerminalAdapterTests(unittest.TestCase):
    def test_collects_multiline_history_as_one_event_with_provenance(self):
        from aetheris.adapters.terminal import TerminalAdapter
        from aetheris.command_privacy import CommandPrivacy

        with tempfile.TemporaryDirectory() as raw:
            history = Path(raw) / "ConsoleHost_history.txt"
            history.write_text(
                "  -Recurse -Filter Microsoft.DriverKit.Build.Tasks.17.0.dll\n"
                'Get-ChildItem "D:\\Program Files (x86)\\Windows Kits\\10\\build"\n',
                encoding="utf-8",
            )

            records = TerminalAdapter(history, syntax_checker=lambda _: True, command_privacy=CommandPrivacy(b"device-key")).collect()

            self.assertEqual(len(records), 1)
            self.assertEqual(records[0]["payload"]["merge_method"], "inferred_parameter_join")
            self.assertEqual(records[0]["provenance"]["source_line_start"], 1)
            self.assertEqual(records[0]["provenance"]["source_line_end"], 2)
            self.assertEqual(records[0]["payload"]["command_type"], "file.search")
            self.assertTrue(records[0]["payload"]["command_hash"].startswith("hmac-sha256:"))

    def test_collects_new_history_lines_and_redacts_secrets(self):
        from aetheris.adapters.terminal import TerminalAdapter

        with tempfile.TemporaryDirectory() as raw:
            history = Path(raw) / "ConsoleHost_history.txt"
            history.write_text("git status\n$env:TOKEN='Bearer abc123'\n", encoding="utf-8")
            adapter = TerminalAdapter(history)
            first = adapter.collect()
            self.assertEqual(len(first), 2)
            self.assertEqual(first[0]["event_type"], "terminal.command")
            self.assertNotIn("abc123", str(first[1]))
            history.write_text(history.read_text(encoding="utf-8") + "git log -1\n", encoding="utf-8")
            second = adapter.collect()
            self.assertEqual([item["payload"]["command"] for item in second], ["git log -1"])


if __name__ == "__main__":
    unittest.main()
