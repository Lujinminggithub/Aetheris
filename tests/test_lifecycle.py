import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch


class LifecycleTests(unittest.TestCase):
    def test_normal_and_fatal_exit_codes_are_distinct(self):
        from aetheris.lifecycle import LifecycleLog, execute_core

        with tempfile.TemporaryDirectory() as raw:
            log = LifecycleLog(Path(raw) / "core.log")
            self.assertEqual(execute_core(lambda: None, log, supervised=True), 0)
            self.assertEqual(execute_core(lambda: (_ for _ in ()).throw(RuntimeError("sensitive body")), log, supervised=True), 1)
            text = (Path(raw) / "core.log").read_text(encoding="utf-8")
            self.assertIn("lifecycle start supervised=true", text)
            self.assertIn("lifecycle stop reason=user_exit", text)
            self.assertIn("lifecycle fatal error=RuntimeError", text)
            self.assertNotIn("sensitive body", text)

    def test_invalid_error_type_is_not_written_and_log_rotates(self):
        from aetheris.lifecycle import LifecycleLog

        with tempfile.TemporaryDirectory() as raw:
            path = Path(raw) / "core.log"
            log = LifecycleLog(path, max_bytes=120, backups=2)
            for _ in range(10):
                log.start(supervised=True)
            log.fatal("ValueError token=must-not-appear")
            content = "\n".join(item.read_text(encoding="utf-8") for item in Path(raw).glob("core.log*"))
            self.assertNotIn("must-not-appear", content)
            self.assertIn("UnknownError", content)
            self.assertTrue((Path(raw) / "core.log.1").is_file())
            self.assertLessEqual(len(list(Path(raw).glob("core.log*"))), 3)

    def test_parser_accepts_supervised_flag(self):
        from aetheris.tray import parse_args

        args = parse_args(["--supervised", "--config", "config.json"])
        self.assertTrue(args.supervised)
        self.assertEqual(args.config, "config.json")
        self.assertIsNone(args.service_session)
        service_args = parse_args(["--service-session", "2", "--config", "config.json"])
        self.assertEqual(service_args.service_session, 2)

    def test_supervised_duplicate_does_not_open_browser(self):
        from aetheris.tray import _handle_existing_instance

        with patch("aetheris.tray.existing_status_url", return_value="http://127.0.0.1:53392/"), \
             patch("aetheris.tray.webbrowser.open") as open_browser, \
             patch("aetheris.tray._show_error") as show_error:
            _handle_existing_instance(Path("config.json"), supervised=True)
        open_browser.assert_not_called()
        show_error.assert_not_called()

    def test_manual_duplicate_opens_existing_lens(self):
        from aetheris.tray import _handle_existing_instance

        with patch("aetheris.tray.existing_status_url", return_value="http://127.0.0.1:53392/"), \
             patch("aetheris.tray.webbrowser.open") as open_browser:
            _handle_existing_instance(Path("config.json"), supervised=False)
        open_browser.assert_called_once_with("http://127.0.0.1:53392/")


if __name__ == "__main__":
    unittest.main()
