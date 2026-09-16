import json
import os
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch


class VSCodeAdapterTests(unittest.TestCase):
    def test_collects_native_code_window_without_bridge_json(self):
        from aetheris.adapters.vscode import VSCodeAdapter, VSCodeWindow

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw) / "sing-box-extern"
            root.mkdir()
            adapter = VSCodeAdapter(
                Path(raw) / "missing-state.vscdb",
                project_roots=[root],
                window_provider=lambda: [VSCodeWindow(15192, "proxy.go - sing-box-extern - Visual Studio Code")],
            )

            records = adapter.collect()

            self.assertEqual(records[0]["event_type"], "ide.activity")
            self.assertEqual(records[0]["payload"]["tool"], "vscode")
            self.assertEqual(records[0]["payload"]["workspace"], "sing-box-extern")
            self.assertEqual(records[0]["payload"]["active_file"], "proxy.go")
            self.assertEqual(records[0]["payload"]["active_file_extension"], ".go")
            self.assertEqual(records[0]["project_path"], str(root.resolve()))
            self.assertNotIn("file_content", str(records))

    def test_unchanged_native_window_is_not_emitted_twice(self):
        from aetheris.adapters.vscode import VSCodeAdapter, VSCodeWindow

        windows = [VSCodeWindow(15192, "proxy.go - repo - Visual Studio Code")]
        adapter = VSCodeAdapter(window_provider=lambda: windows)

        self.assertEqual(len(adapter.collect()), 1)
        self.assertEqual(adapter.collect(), [])
        windows[0] = VSCodeWindow(15192, "main.go - repo - Visual Studio Code")
        self.assertEqual(adapter.collect()[0]["payload"]["active_file"], "main.go")

    def test_duplicate_workspace_names_prefer_primary_project_root(self):
        from aetheris.adapters.vscode import VSCodeAdapter, VSCodeWindow

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            worktree = root / ".worktrees" / "branch" / "repo"
            primary = root / "repo"
            worktree.mkdir(parents=True)
            primary.mkdir()
            adapter = VSCodeAdapter(
                project_roots=[worktree, primary],
                window_provider=lambda: [VSCodeWindow(1, "main.go - repo - Visual Studio Code")],
            )

            record = adapter.collect()[0]

            self.assertEqual(record["project_path"], str(primary.resolve()))

    def test_default_state_path_uses_real_vscode_database(self):
        from aetheris.adapters.vscode import VSCodeAdapter

        with patch.dict(os.environ, {"APPDATA": r"C:\Users\dev\AppData\Roaming"}):
            self.assertEqual(
                VSCodeAdapter.default_state_path(),
                Path(r"C:\Users\dev\AppData\Roaming\Code\User\globalStorage\state.vscdb"),
            )

    def test_reads_workspace_activity_without_file_contents(self):
        from aetheris.adapters.vscode import VSCodeAdapter

        with tempfile.TemporaryDirectory() as raw:
            state = Path(raw) / "state.json"
            state.write_text(json.dumps({"workspace": "repo", "active_file": "app.py", "last_command": "pytest"}), encoding="utf-8")
            records = VSCodeAdapter(state).collect()
            self.assertEqual(records[0]["event_type"], "ide.activity")
            self.assertEqual(records[0]["payload"]["active_file"], "app.py")
            self.assertNotIn("print(", str(records))


if __name__ == "__main__":
    unittest.main()
