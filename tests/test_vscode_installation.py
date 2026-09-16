import json
import subprocess
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch


class VSCodeInstallationTests(unittest.TestCase):
    def test_install_launches_code_directly_without_shell(self):
        from aetheris.vscode_installation import VSCodeInstallation

        calls = []
        def runner(argv, **kwargs):
            calls.append((argv, kwargs))
            return subprocess.CompletedProcess(argv, 0, "", "")

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            code = root / "Code.exe"; code.write_bytes(b"code")
            cli = root / "resources" / "app" / "out" / "cli.js"; cli.parent.mkdir(parents=True); cli.write_text("cli", encoding="utf-8")
            vsix = root / "Aetheris.vsix"; vsix.write_bytes(b"vsix")
            installation = VSCodeInstallation(root / "state.json", vsix, code_path=code, runner=runner)

            result = installation.install()

            self.assertEqual(result.state, "awaiting_activation")
            self.assertEqual(calls[0][0], [str(code), str(cli), "--install-extension", str(vsix), "--force"])
            self.assertFalse(calls[0][1]["shell"])
            self.assertTrue(calls[0][1]["creationflags"] & subprocess.CREATE_NO_WINDOW)
            self.assertEqual(calls[0][1]["env"]["ELECTRON_RUN_AS_NODE"], "1")
            self.assertLessEqual(calls[0][1]["timeout"], 30)

    def test_timeout_is_persisted_and_never_escapes_core_startup(self):
        from aetheris.vscode_installation import VSCodeInstallation

        now = [1000.0]
        def runner(argv, **kwargs):
            raise subprocess.TimeoutExpired(argv, kwargs["timeout"])

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            code = root / "Code.exe"; code.write_bytes(b"code")
            cli = root / "resources" / "app" / "out" / "cli.js"; cli.parent.mkdir(parents=True); cli.write_text("cli", encoding="utf-8")
            vsix = root / "Aetheris.vsix"; vsix.write_bytes(b"vsix")
            installation = VSCodeInstallation(root / "state.json", vsix, code_path=code, runner=runner, clock=lambda: now[0])

            result = installation.install()

            self.assertEqual((result.state, result.reason_code), ("error", "extension_install_timeout"))
            self.assertEqual(installation.inspect().reason_code, "extension_install_timeout")
            with patch.object(installation, "_installed_version", return_value=""):
                self.assertFalse(installation.should_auto_install())
                now[0] += 300
                self.assertTrue(installation.should_auto_install())

    def test_user_pause_and_decline_are_persisted(self):
        from aetheris.vscode_installation import VSCodeInstallation

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            installation = VSCodeInstallation(root / "state.json", root / "Aetheris.vsix")
            installation.decline()
            self.assertEqual(installation.inspect().state, "install_declined")
            installation.pause()
            self.assertEqual(VSCodeInstallation(root / "state.json", root / "Aetheris.vsix").inspect().state, "paused_by_user")
            self.assertNotIn("token", json.dumps(installation.inspect().__dict__))

    def test_uninstall_does_not_trigger_automatic_reinstall(self):
        from aetheris.vscode_installation import VSCodeInstallation

        calls = []
        def runner(argv, **kwargs):
            calls.append(argv)
            return subprocess.CompletedProcess(argv, 0, "", "")
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            code = root / "Code.exe"; code.write_bytes(b"code")
            cli = root / "resources" / "app" / "out" / "cli.js"; cli.parent.mkdir(parents=True); cli.write_text("cli", encoding="utf-8")
            installation = VSCodeInstallation(root / "state.json", root / "Aetheris.vsix", code_path=code, runner=runner)
            result = installation.uninstall()
            self.assertEqual(result.state, "not_installed")
            self.assertEqual(calls, [[str(code), str(cli), "--uninstall-extension", "aetheris.aetheris-vscode"]])
            self.assertEqual(installation.inspect().state, "not_installed")

    def test_tray_constructor_does_not_install_optional_extension(self):
        from aetheris.tray import TrayConfig, TrayCore

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw); (root / "config").mkdir(); (root / "data").mkdir(); (root / "integrations").mkdir()
            config_path = root / "config" / "aetheris.json"; config_path.write_text("{}", encoding="utf-8")
            token = root / "token"; token.write_text("AETHERIS_TOKEN=device-token\n", encoding="utf-8")
            (root / "integrations" / "AetherisVSCode.vsix").write_bytes(b"vsix")
            config = TrayConfig("http://server", token, None, root / "data" / "client.db", config_path=config_path, vscode_state_path=root / "missing.vscdb")

            with patch("aetheris.tray.VSCodeInstallation.install") as install, patch("aetheris.tray.VSCodeInstallation._installed_version", return_value=""):
                core = TrayCore(config)
            try:
                install.assert_not_called()
                with patch.object(core.vscode_installation, "_installed_version", return_value=""):
                    self.assertEqual(core.vscode_installation.inspect().state, "not_installed")
            finally:
                core.stop()

    def test_tray_projects_only_safe_recent_vscode_fields(self):
        from aetheris.tray import TrayCore

        core = TrayCore.__new__(TrayCore)
        core.queue = SimpleNamespace(history=lambda limit=200: [{
            "source": "core.vscode.extension", "event_type": "ide.file_saved",
            "occurred_at": "2026-09-14T08:00:00Z", "project_id": "project-1",
            "payload": {"relative_path": "src/app.py", "language_id": "python", "inserted_chars": 900},
            "provenance": {"private": "must-not-leak"},
        }, {"source": "core.git", "event_type": "git.commit", "payload": {}}])
        core.project_registry = None

        events = core.vscode_recent_events()

        self.assertEqual(events, [{
            "event_type": "ide.file_saved", "occurred_at": "2026-09-14T08:00:00Z",
            "project_id": "project-1", "project_name": "project-1",
            "relative_path": "src/app.py", "language_id": "python",
        }])
        self.assertNotIn("inserted_chars", str(events))


if __name__ == "__main__":
    unittest.main()
